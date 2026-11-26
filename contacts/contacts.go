// Package contacts loads a local contacts book (a CSV file with number,name
// rows) and maps caller numbers to display names. It is a leaf package:
// stdlib only, no app-package imports, no cgo. The file is a convenience,
// not a security boundary: a missing or malformed file degrades to "no
// names" with a warning, an unreadable file keeps the last-good names —
// never a startup error.
package contacts

import (
	"bytes"
	"cmp"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// maxFileSize bounds the reload: anything larger is treated as malformed.
const maxFileSize = 1 << 20

// maxLabelRunes bounds a rendered contacts label. Display only: the lookup
// keys stay raw and unbounded.
const maxLabelRunes = 128

// Resolve turns the configured contacts path into an absolute file path.
// It never errors and never creates anything (read-only os.Stat probing).
// With a configured path it just makes it absolute; otherwise it picks the
// first existing regular contacts.csv among the search candidates and, when
// none is found, a creation target: CWD first, then the user config dir,
// then the executable dir (under `go run` the exe dir is ephemeral). Each
// fallback applies only when the previous base dir cannot be DETERMINED
// (Getwd/UserConfigDir/Executable error) — it is not a retry on permission
// failure; a permission-denied creation later just errors in EnsureFile
// and is logged.
func Resolve(configured string) string {
	if configured != "" {
		if abs, err := filepath.Abs(configured); err == nil {
			return abs
		}
		return configured
	}
	for _, c := range searchCandidates() {
		if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() {
			return c
		}
	}
	// Nothing found: pick where EnsureFile would create the file.
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "contacts.csv")
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "contacts.csv")
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "contacts.csv")
	}
	return "contacts.csv"
}

// searchCandidates mirrors config.searchCandidates with the contacts
// filename (deliberate duplication — config stays untouched): next to the
// binary, in the CWD, in the user config dir. Candidates whose base
// directory cannot be determined are skipped.
func searchCandidates() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "contacts.csv"))
	}
	if wd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(wd, "contacts.csv"))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, "contacts.csv"))
	}
	return out
}

// Book is a lazily loaded view of the contacts CSV, re-read per call only
// when the file's mtime/size signature changes. A nil *Book is valid and
// behaves as an empty book: Path "", Lookup "", Display unchanged,
// Version 0, EnsureFile/Open return an error.
type Book struct {
	path string // resolved path, immutable after New

	mu         sync.Mutex // guards everything below
	names      map[string]string
	mtime      time.Time
	size       int64
	exists     bool
	loaded     bool
	warnedStat bool // one-shot guard for the never-loaded stat-failure WARN
	ver        uint64
}

// New returns a Book for the resolved path. It performs no I/O; the file is
// read on the first Lookup/Display/Version call.
func New(path string) *Book {
	return &Book{path: path}
}

// Path returns the resolved contacts file path ("" on a nil book).
func (b *Book) Path() string {
	if b == nil {
		return ""
	}
	return b.path
}

// Lookup returns the name for an exact number match, or "" when unknown.
func (b *Book) Lookup(number string) string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()
	return b.names[number]
}

// Display formats a number for user-facing text: "Name (number)" when a
// name is known, the number unchanged otherwise.
func (b *Book) Display(number string) string {
	if name := b.Lookup(number); name != "" {
		return fmt.Sprintf("%s (%s)", name, number)
	}
	return number
}

// Entry is one contacts-book row: the raw exact-match number and the
// sanitized display name.
type Entry struct {
	Number string
	Name   string
}

// Label renders the entry for display: "Name — number" (em dash, matching
// the history labels), the number when the name is empty, the name when the
// number is empty. Display-only hygiene: control characters in either part
// fold to single spaces and the result is trimmed; XML-unsafe characters are
// deliberately kept (a tray label is never interpolated into a toast body).
// The final label is capped at maxLabelRunes runes; lookup keys stay raw.
func (e Entry) Label() string {
	name := displayText(e.Name)
	number := displayText(e.Number)
	switch {
	case number == "":
		return name
	case name == "":
		return number
	}
	label := name + " — " + number
	if runes := []rune(label); len(runes) > maxLabelRunes {
		label = string(runes[:maxLabelRunes])
	}
	return label
}

// entryRow is the sortable form of one book row: the entry plus its
// lowercased name, filled in outside the book mutex.
type entryRow struct {
	entry     Entry
	lowerName string
}

// snapshotRows copies the current names map into rows with the mutex held.
// lowerName stays empty — the caller fills it outside the lock so the
// notify path never waits on string processing. A nil book yields nil.
func (b *Book) snapshotRows() []entryRow {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()
	if len(b.names) == 0 {
		return nil
	}
	rows := make([]entryRow, 0, len(b.names))
	for number, name := range b.names {
		rows = append(rows, entryRow{entry: Entry{Number: number, Name: name}})
	}
	return rows
}

// Entries returns a sorted snapshot of the book: name ascending,
// case-insensitive, ties broken by number ascending (byte order). It is a
// defensive copy (a fresh slice on every call); a nil book, a missing file
// or a malformed file yields nil. The map is copied under the lock; the
// lowercasing and the sort happen outside it, so a concurrent notify-path
// Lookup never waits on either.
func (b *Book) Entries() []Entry {
	rows := b.snapshotRows()
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].lowerName = strings.ToLower(rows[i].entry.Name)
	}
	slices.SortFunc(rows, func(x, y entryRow) int {
		if n := cmp.Compare(x.lowerName, y.lowerName); n != 0 {
			return n
		}
		return cmp.Compare(x.entry.Number, y.entry.Number)
	})
	entries := make([]Entry, len(rows))
	for i, r := range rows {
		entries[i] = r.entry
	}
	return entries
}

// SlotLabels returns the display labels of the first max entries (in
// Entries order), or nil when max is not positive or the book is empty.
// This is the only contacts API the Contacts submenu uses for its labels.
func (b *Book) SlotLabels(max int) []string {
	if max <= 0 {
		return nil
	}
	entries := b.Entries()
	if len(entries) == 0 {
		return nil
	}
	if max > len(entries) {
		max = len(entries)
	}
	labels := make([]string, max)
	for i := range labels {
		labels[i] = entries[i].Label()
	}
	return labels
}

// displayText folds control-character runs to a single space and trims the
// result. Display-only: unlike sanitizeName it keeps XML-unsafe characters.
func displayText(s string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range s {
		if unicode.IsControl(r) {
			pendingSpace = true
			continue
		}
		if pendingSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		pendingSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// Version counts observed file changes (every reload, including deletion
// and parse-failure degradation). It refreshes the cache as a side effect.
func (b *Book) Version() uint64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()
	return b.ver
}

// refresh re-stats the file and reloads only when the mtime/size signature
// changed. Called with b.mu held. Fail-open: on any problem the book keeps
// the last-good names (read errors) or degrades to no names (missing or
// malformed file), logging at most once per observed change — plus a
// one-shot WARN when the file has never loaded and stat keeps failing.
func (b *Book) refresh() {
	st, err := os.Stat(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			if !b.loaded || b.exists {
				b.names = nil
				b.exists = false
				b.loaded = true
				b.ver++
			}
		} else if !b.loaded && !b.warnedStat {
			// Never loaded and stat fails (e.g. permissions): warn once,
			// not on every lookup.
			b.warnedStat = true
			log.Printf("WARN: cannot access contacts file %s: %v", b.path, err)
		}
		// Other stat errors: keep the last-good cache.
		return
	}
	if b.loaded && b.exists && st.ModTime() == b.mtime && st.Size() == b.size {
		return
	}
	// File appeared or changed: cache the new signature and count the
	// change even when the resulting map ends up identical.
	b.mtime = st.ModTime()
	b.size = st.Size()
	b.exists = true
	b.loaded = true
	b.ver++

	if st.Size() > maxFileSize {
		b.names = nil
		log.Printf("WARN: contacts file %s is malformed: %v; ignoring contacts", b.path, fmt.Errorf("file too large (%d bytes)", st.Size()))
		return
	}
	data, err := os.ReadFile(b.path)
	if err != nil {
		// Keep the last-good names (e.g. an Excel sharing violation). The
		// new signature is cached, so no retry/log until the file changes.
		log.Printf("WARN: cannot read contacts file %s: %v; keeping previous contacts", b.path, err)
		return
	}
	names, err := parse(data)
	if err != nil {
		b.names = nil
		log.Printf("WARN: contacts file %s is malformed: %v; ignoring contacts", b.path, err)
		return
	}
	b.names = names
}

// parse reads CSV contents into a number→name map. Pure and all-or-nothing:
// invalid UTF-8 or any CSV error discards everything. A leading UTF-8 BOM
// (Excel) is stripped; a "number,name" header row is skipped; rows with an
// empty number or name are skipped (names are sanitized first, see
// sanitizeName); extra fields are ignored; on duplicate numbers the last
// valid row wins. Numbers are NOT sanitized: they are exact-match keys,
// and the caller side already sanitizes the lookup key.
func parse(data []byte) (map[string]string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) {
		return nil, errors.New("invalid UTF-8")
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	names := make(map[string]string)
	for i, rec := range records {
		if i == 0 && len(rec) >= 2 &&
			strings.EqualFold(strings.TrimSpace(rec[0]), "number") &&
			strings.EqualFold(strings.TrimSpace(rec[1]), "name") {
			continue // header row
		}
		if len(rec) == 0 {
			continue
		}
		number := strings.TrimSpace(rec[0])
		if number == "" || len(rec) < 2 {
			continue
		}
		name := sanitizeName(rec[1])
		if name == "" {
			continue
		}
		names[number] = name
	}
	return names, nil
}

// sanitizeName makes a contact name safe for notification bodies: beeep's
// Windows toast path interpolates the body into unescaped XML CDATA (a
// "]]>" would break out) and a PowerShell here-string (a newline followed
// by a line starting "@ would terminate it). Control characters (incl.
// \r\n\t) fold to a single space (runs collapse, result trimmed), the
// XML-unsafe set is stripped (same as core.sanitizeCaller), and the result
// is capped at 64 runes (mirroring the caller cap).
func sanitizeName(name string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range name {
		switch {
		case unicode.IsControl(r):
			pendingSpace = true // collapse control runs to one space
		case r == '<' || r == '>' || r == '&' || r == '"' || r == '\'':
			// strip the XML-unsafe set
		default:
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
		}
	}
	runes := []rune(strings.TrimSpace(b.String()))
	if len(runes) > 64 {
		runes = runes[:64]
	}
	return string(runes)
}

// EnsureFile creates the contacts file and its parent directories when
// missing. Idempotent: an existing regular file is never truncated or
// modified. A new file gets a UTF-8 BOM + "number,name\r\n" header so Excel
// opens it cleanly. Least privilege: the file is created 0o600 and parent
// dirs 0o700 — the book holds personal data (names + numbers).
func (b *Book) EnsureFile() error {
	if b == nil {
		return errors.New("no contacts book configured")
	}
	if st, err := os.Stat(b.path); err == nil {
		if st.Mode().IsRegular() {
			return nil
		}
		return fmt.Errorf("%s is not a regular file", b.path)
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(b.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil // created concurrently between Stat and Open: fine
		}
		return err
	}
	if _, err := f.WriteString("\xEF\xBB\xBFnumber,name\r\n"); err != nil {
		f.Close()
		os.Remove(b.path) // do not leave a partial stub behind
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(b.path)
		return err
	}
	return nil
}

// Open launches the OS default application for the contacts file. It does
// not ensure the file exists — call EnsureFile first.
func (b *Book) Open() error {
	if b == nil {
		return errors.New("no contacts book configured")
	}
	return openFile(b.path)
}

package contacts

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func writeContactsFile(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "contacts.csv")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", p, err)
	}
	return p
}

// bookOver writes content to a fresh temp contacts.csv and returns a Book
// over it (real file I/O, no mocks — the package is stdlib-only).
func bookOver(t *testing.T, content string) *Book {
	t.Helper()
	return New(writeContactsFile(t, t.TempDir(), content))
}

// chdirIsolated points the CWD and the user config dir at fresh empty
// locations under a temp dir (hermetic, per-OS env names like
// config_test.go). HOME is set too: on macOS os.UserConfigDir ignores
// XDG_CONFIG_HOME and derives the dir from $HOME, so the actual user
// config dir is read back AFTER setting the env and created if needed.
func chdirIsolated(t *testing.T) (cwd, ucfg string) {
	t.Helper()
	d := t.TempDir()
	cwd = filepath.Join(d, "cwd")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", cwd, err)
	}
	t.Setenv("APPDATA", filepath.Join(d, "userconfig"))         // Windows
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(d, "userconfig")) // Linux
	t.Setenv("HOME", d)                                         // macOS: $HOME/Library/Application Support
	var err error
	ucfg, err = os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	if err := os.MkdirAll(ucfg, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", ucfg, err)
	}
	t.Chdir(cwd)
	return cwd, ucfg
}

func absPath(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("filepath.Abs(%s): %v", p, err)
	}
	return abs
}

// --- New / nil book ---

func TestNewPerformsNoIO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "contacts.csv")
	b := New(p)
	if b.Path() != p {
		t.Errorf("Path() = %q, want %q", b.Path(), p)
	}
	// No load, no stat, no version bump, no file creation.
	if b.loaded || b.exists || b.names != nil || b.ver != 0 {
		t.Errorf("New must not touch the file: loaded=%v exists=%v names=%v ver=%d",
			b.loaded, b.exists, b.names, b.ver)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("New must not create the file (stat err = %v)", err)
	}
}

func TestNilBookIsSafe(t *testing.T) {
	var b *Book
	if got := b.Path(); got != "" {
		t.Errorf("nil Path = %q, want \"\"", got)
	}
	if got := b.Lookup("144"); got != "" {
		t.Errorf("nil Lookup = %q, want \"\"", got)
	}
	if got := b.Display("144"); got != "144" {
		t.Errorf("nil Display = %q, want the number unchanged", got)
	}
	if got := b.Display(""); got != "" {
		t.Errorf("nil Display(\"\") = %q, want \"\"", got)
	}
	if got := b.Version(); got != 0 {
		t.Errorf("nil Version = %d, want 0", got)
	}
	if err := b.EnsureFile(); err == nil || err.Error() != "no contacts book configured" {
		t.Errorf("nil EnsureFile = %v, want \"no contacts book configured\"", err)
	}
	if err := b.Open(); err == nil || err.Error() != "no contacts book configured" {
		t.Errorf("nil Open = %v, want \"no contacts book configured\"", err)
	}
}

// --- Lookup / Display ---

func TestLookupExactMatchOnly(t *testing.T) {
	b := bookOver(t, "144,Alice\n+79991234567,Bob\n")
	tests := []struct {
		number string
		want   string
	}{
		{"144", "Alice"},
		{"+79991234567", "Bob"},
		{"145", ""},   // unknown number
		{"14", ""},    // no prefix matching
		{" 144", ""},  // query is NOT trimmed: exact match only
		{"Alice", ""}, // names are not keys
	}
	for _, tt := range tests {
		if got := b.Lookup(tt.number); got != tt.want {
			t.Errorf("Lookup(%q) = %q, want %q", tt.number, got, tt.want)
		}
	}
}

func TestDisplayKnownAndUnknown(t *testing.T) {
	b := bookOver(t, "144,Alice\n")
	if got := b.Display("144"); got != "Alice (144)" {
		t.Errorf("Display(known) = %q, want %q", got, "Alice (144)")
	}
	if got := b.Display("999"); got != "999" {
		t.Errorf("Display(unknown) = %q, want the number unchanged", got)
	}
	if got := b.Display(""); got != "" {
		t.Errorf("Display(\"\") = %q, want \"\"", got)
	}
}

// --- parse (exercised via Lookup on real temp files) ---

func TestParseViaLookup(t *testing.T) {
	// Exactly 1 MiB: the size cap is ">", so this must still load.
	prefix := "144,Alice\n"
	exactMiB := prefix + "999," + strings.Repeat("x", (1<<20)-len(prefix)-4-1) + "\n"

	tests := []struct {
		name    string
		content string
		number  string
		want    string
	}{
		{name: "header skipped", content: "number,name\n144,Alice\n", number: "144", want: "Alice"},
		{name: "header case-insensitive and trimmed", content: " Number , NAME \n144,Alice\n", number: "144", want: "Alice"},
		{name: "header with extra columns skipped", content: "number,name,extra\n144,Alice\n", number: "144", want: "Alice"},
		{name: "header-like row only skipped in first position", content: "144,Alice\nnumber,name\n", number: "number", want: "name"},
		{name: "BOM stripped with header", content: "\xEF\xBB\xBFnumber,name\n144,Alice\n", number: "144", want: "Alice"},
		{name: "BOM stripped without header", content: "\xEF\xBB\xBF144,Alice\n", number: "144", want: "Alice"},
		{name: "CRLF rows", content: "number,name\r\n144,Alice\r\n", number: "144", want: "Alice"},
		{name: "extra columns ignored", content: "144,Alice,extra,columns\n", number: "144", want: "Alice"},
		{name: "number-only row ignored", content: "144\n145,Bob\n", number: "144", want: ""},
		{name: "lazy quotes tolerated, unsafe quote stripped", content: "144,Ali\"ce\n", number: "144", want: "Alice"},
		{name: "spaces trimmed", content: " 144 , Alice \n", number: "144", want: "Alice"},
		{name: "empty rows skipped", content: "\n\n144,Alice\n\n", number: "144", want: "Alice"},
		{name: "empty number skipped", content: ",Alice\n", number: "", want: ""},
		{name: "whitespace-only number skipped", content: "   ,Alice\n", number: "", want: ""},
		{name: "empty name skipped", content: "144,\n", number: "144", want: ""},
		{name: "whitespace-only name skipped", content: "144,   \n", number: "144", want: ""},
		{name: "duplicate numbers last valid wins", content: "144,Alice\n144,Bob\n", number: "144", want: "Bob"},
		{name: "invalid duplicate does not clobber", content: "144,Alice\n144,\n144\n", number: "144", want: "Alice"},
		{name: "quoted name with comma", content: "144,\"Doe, John\"\n", number: "144", want: "Doe, John"},
		// Name sanitization: notification bodies go into unescaped XML
		// CDATA and a PowerShell here-string on Windows (see sanitizeName).
		{name: "CDATA breaker stripped", content: "144,A]]>B\n", number: "144", want: "A]]B"},
		{name: "here-string breaker folded and stripped", content: "144,\"Evil\n\"\"@echo pwned\"\n", number: "144", want: "Evil @echo pwned"},
		{name: "control chars fold to single spaces", content: "144,\"A\tB\r\nC\"\n", number: "144", want: "A B C"},
		{name: "control-only name skipped", content: "144,\"\r\n\t\"\n", number: "144", want: ""},
		{name: "name capped at 64 runes", content: "144," + strings.Repeat("x", 70) + "\n", number: "144", want: strings.Repeat("x", 64)},
		{name: "name cap counts runes not bytes", content: "144," + strings.Repeat("é", 70) + "\n", number: "144", want: strings.Repeat("é", 64)},
		{name: "XML-unsafe-only name skipped", content: "144,<>&\"'\n", number: "144", want: ""},
		{name: "numbers are not sanitized (exact-match keys)", content: "1<44,Alice\n", number: "1<44", want: "Alice"},
		{name: "empty file", content: "", number: "144", want: ""},
		{name: "invalid UTF-8 discards everything", content: "144,Alice\n145,\xFF\xFE\n", number: "144", want: ""},
		// NOTE: the spec also says "unterminated quote → empty map", but
		// LazyQuotes (also spec-mandated) makes encoding/csv tolerate an
		// unterminated quote at EOF: the row is salvaged and earlier valid
		// rows survive. Fail-open still holds (no panic, no startup error).
		// See the test report for the spec conflict.
		{name: "unterminated quote tolerated by LazyQuotes", content: "144,Alice\n145,\"Bob\n", number: "144", want: "Alice"},
		{name: "oversize file discards everything", content: "144,Alice\n" + strings.Repeat("x", 1<<20) + ",y\n", number: "144", want: ""},
		{name: "exactly 1 MiB is accepted", content: exactMiB, number: "144", want: "Alice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bookOver(t, tt.content)
			if got := b.Lookup(tt.number); got != tt.want {
				t.Errorf("Lookup(%q) = %q, want %q", tt.number, got, tt.want)
			}
		})
	}
}

// --- refresh / Version ---

func TestVersionCountsObservedChanges(t *testing.T) {
	p := filepath.Join(t.TempDir(), "contacts.csv")
	b := New(p)

	// Missing file: the first observation counts as a change, then stable.
	if got := b.Lookup("144"); got != "" {
		t.Errorf("Lookup on missing file = %q, want \"\"", got)
	}
	if got := b.Version(); got != 1 {
		t.Errorf("Version after first observation of missing file = %d, want 1", got)
	}
	if got := b.Version(); got != 1 {
		t.Errorf("Version on repeated reads of missing file = %d, want stable 1", got)
	}

	// File appears.
	writeContactsFile(t, filepath.Dir(p), "144,Alice\n")
	if got := b.Lookup("144"); got != "Alice" {
		t.Errorf("Lookup after file appeared = %q, want %q", got, "Alice")
	}
	if got := b.Version(); got != 2 {
		t.Errorf("Version after file appeared = %d, want 2", got)
	}
	// Stable while unchanged.
	if got := b.Version(); got != 2 {
		t.Errorf("Version on unchanged file = %d, want stable 2", got)
	}

	// Rewrite with different content (different size: signature changes
	// regardless of mtime granularity).
	writeContactsFile(t, filepath.Dir(p), "144,Bob\n222,Carol\n")
	if got := b.Lookup("144"); got != "Bob" {
		t.Errorf("Lookup after rewrite = %q, want %q", got, "Bob")
	}
	if got := b.Lookup("222"); got != "Carol" {
		t.Errorf("Lookup(new number) after rewrite = %q, want %q", got, "Carol")
	}
	if got := b.Version(); got != 3 {
		t.Errorf("Version after rewrite = %d, want 3", got)
	}

	// Deletion degrades to no names and counts.
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := b.Lookup("144"); got != "" {
		t.Errorf("Lookup after deletion = %q, want \"\"", got)
	}
	if got := b.Version(); got != 4 {
		t.Errorf("Version after deletion = %d, want 4", got)
	}
	if got := b.Version(); got != 4 {
		t.Errorf("Version on repeated reads after deletion = %d, want stable 4", got)
	}

	// Re-appearance counts again.
	writeContactsFile(t, filepath.Dir(p), "144,Alice\n")
	if got := b.Lookup("144"); got != "Alice" {
		t.Errorf("Lookup after re-appearance = %q, want %q", got, "Alice")
	}
	if got := b.Version(); got != 5 {
		t.Errorf("Version after re-appearance = %d, want 5", got)
	}
}

func TestRefreshMtimeOnlyChangeCounts(t *testing.T) {
	p := writeContactsFile(t, t.TempDir(), "144,Alice\n")
	b := New(p)
	if got := b.Lookup("144"); got != "Alice" {
		t.Fatalf("initial Lookup = %q, want %q", got, "Alice")
	}
	v := b.Version()

	st, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	future := st.ModTime().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if got := b.Version(); got != v+1 {
		t.Errorf("Version after mtime-only change = %d, want %d", got, v+1)
	}
	if got := b.Lookup("144"); got != "Alice" {
		t.Errorf("Lookup after mtime-only change = %q, want %q (same content)", got, "Alice")
	}
}

func TestRefreshSkipsReloadOnUnchangedSignature(t *testing.T) {
	p := writeContactsFile(t, t.TempDir(), "144,Alice\n")
	b := New(p)
	if got := b.Lookup("144"); got != "Alice" {
		t.Fatalf("initial Lookup = %q, want %q", got, "Alice")
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Same size, different content, but the exact mtime/size signature is
	// restored: the cache must NOT reload (proves stat-gated reloading).
	writeContactsFile(t, filepath.Dir(p), "144,Bobby\n") // also 10 bytes
	if err := os.Chtimes(p, st.ModTime(), st.ModTime()); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if got := b.Lookup("144"); got != "Alice" {
		t.Errorf("Lookup with unchanged signature = %q, want cached %q", got, "Alice")
	}
	if got := b.Version(); got != 1 {
		t.Errorf("Version with unchanged signature = %d, want stable 1", got)
	}
}

func TestRefreshReadErrorKeepsLastGood(t *testing.T) {
	p := writeContactsFile(t, t.TempDir(), "144,Alice\n")
	b := New(p)
	if got := b.Lookup("144"); got != "Alice" {
		t.Fatalf("initial Lookup = %q, want %q", got, "Alice")
	}
	v := b.Version()

	// Swap the file for a DIRECTORY of the same name: os.Stat succeeds (the
	// signature changes) but os.ReadFile fails on every OS — the read-error
	// path must keep the last-good names and still count the change.
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := b.Lookup("144"); got != "Alice" {
		t.Errorf("Lookup after read error = %q, want last-good %q", got, "Alice")
	}
	if got := b.Version(); got != v+1 {
		t.Errorf("Version after read error = %d, want %d", got, v+1)
	}
}

func TestRefreshStatErrorNeverLoadedWarnsOnce(t *testing.T) {
	// A NUL in the path makes os.Stat fail with a non-IsNotExist error on
	// every OS — portable, unlike ACL denial (not enforceable on Windows).
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	b := New(filepath.Join(t.TempDir(), "bad\x00name.csv"))
	if got := b.Lookup("144"); got != "" {
		t.Errorf("Lookup on inaccessible file = %q, want \"\"", got)
	}
	b.Lookup("144")
	b.Version()
	if n := strings.Count(buf.String(), "WARN: cannot access contacts file"); n != 1 {
		t.Errorf("stat-failure WARN count = %d, want exactly 1 (one-shot)", n)
	}
}

func TestRefreshParseFailureDegradesAndCounts(t *testing.T) {
	p := writeContactsFile(t, t.TempDir(), "144,Alice\n")
	b := New(p)
	if got := b.Lookup("144"); got != "Alice" {
		t.Fatalf("initial Lookup = %q, want %q", got, "Alice")
	}

	// Invalid UTF-8 (different size: signature changes).
	writeContactsFile(t, filepath.Dir(p), "144,\xFF\n")
	if got := b.Lookup("144"); got != "" {
		t.Errorf("Lookup after malformed rewrite = %q, want \"\" (degraded)", got)
	}
	if got := b.Version(); got != 2 {
		t.Errorf("Version after parse-failure degradation = %d, want 2", got)
	}

	// Oversize file degrades the same way.
	writeContactsFile(t, filepath.Dir(p), "144,Alice\n"+strings.Repeat("x", 1<<20)+",y\n")
	if got := b.Lookup("144"); got != "" {
		t.Errorf("Lookup after oversize rewrite = %q, want \"\" (degraded)", got)
	}
	if got := b.Version(); got != 3 {
		t.Errorf("Version after oversize degradation = %d, want 3", got)
	}

	// Recovery: a valid rewrite loads again.
	writeContactsFile(t, filepath.Dir(p), "144,Alice\n")
	if got := b.Lookup("144"); got != "Alice" {
		t.Errorf("Lookup after recovery = %q, want %q", got, "Alice")
	}
	if got := b.Version(); got != 4 {
		t.Errorf("Version after recovery = %d, want 4", got)
	}
}

// --- Resolve ---

func TestResolveExplicitRelativeBecomesAbsolute(t *testing.T) {
	d := t.TempDir()
	t.Chdir(d)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	got := Resolve("book.csv")
	want := filepath.Join(wd, "book.csv")
	if got != want {
		t.Fatalf("Resolve(relative) = %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("Resolve(relative) = %q, want an absolute path", got)
	}
}

func TestResolveExplicitMissingIsNotSearchedOrCreated(t *testing.T) {
	d := t.TempDir()
	t.Chdir(d)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// A discoverable contacts.csv exists in the CWD: the explicit path must
	// still win, and nothing may be created for it.
	writeContactsFile(t, d, "144,Alice\n")
	got := Resolve(filepath.Join("missing", "book.csv"))
	want := filepath.Join(wd, "missing", "book.csv")
	if got != want {
		t.Fatalf("Resolve(explicit missing) = %q, want %q (no search fallback)", got, want)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Errorf("Resolve must not create the explicit path (stat err = %v)", err)
	}
}

func TestResolveExplicitAbsolutePassesThrough(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "sub", "book.csv")
	if got := Resolve(abs); got != abs {
		t.Fatalf("Resolve(absolute) = %q, want %q", got, abs)
	}
}

func TestResolveSearchCWDThenUserConfigDir(t *testing.T) {
	cwd, ucfg := chdirIsolated(t)

	// Nothing in the CWD: the user config dir file wins.
	ucfgFile := writeContactsFile(t, ucfg, "222,Bob\n")
	if got := Resolve(""); got != absPath(t, ucfgFile) {
		t.Errorf("Resolve(\"\") = %q, want the user config dir file %q", got, ucfgFile)
	}

	// A CWD file beats the user config dir file.
	cwdFile := writeContactsFile(t, cwd, "144,Alice\n")
	if got := Resolve(""); got != absPath(t, cwdFile) {
		t.Errorf("Resolve(\"\") = %q, want the CWD file %q", got, cwdFile)
	}
}

func TestResolveSearchSkipsDirectoryCandidate(t *testing.T) {
	cwd, ucfg := chdirIsolated(t)
	// A DIRECTORY named contacts.csv in the CWD: present, but not regular.
	if err := os.Mkdir(filepath.Join(cwd, "contacts.csv"), 0o755); err != nil {
		t.Fatalf("mkdir dir-candidate: %v", err)
	}
	ucfgFile := writeContactsFile(t, ucfg, "222,Bob\n")
	if got := Resolve(""); got != absPath(t, ucfgFile) {
		t.Fatalf("directory candidate must be skipped: got %q, want %q", got, ucfgFile)
	}
}

func TestResolveSearchNoneFoundReturnsCreationTarget(t *testing.T) {
	chdirIsolated(t) // empty CWD, empty user config dir
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	got := Resolve("")
	want := filepath.Join(wd, "contacts.csv")
	if got != want {
		t.Fatalf("Resolve(\"\") with nothing found = %q, want creation target %q", got, want)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Errorf("Resolve must not create the fallback target (stat err = %v)", err)
	}
}

func TestSearchCandidatesOrderAndContents(t *testing.T) {
	var want []string
	if exe, err := os.Executable(); err == nil {
		want = append(want, filepath.Join(filepath.Dir(exe), "contacts.csv"))
	}
	if wd, err := os.Getwd(); err == nil {
		want = append(want, filepath.Join(wd, "contacts.csv"))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		want = append(want, filepath.Join(dir, "contacts.csv"))
	}
	got := searchCandidates()
	if !slices.Equal(got, want) {
		t.Fatalf("candidates:\n got  %v\n want %v", got, want)
	}
}

// --- EnsureFile ---

func TestEnsureFileCreatesParentsAndHeader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "b", "contacts.csv")
	b := New(p)
	if err := b.EnsureFile(); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading created file: %v", err)
	}
	want := "\xEF\xBB\xBFnumber,name\r\n"
	if string(data) != want {
		t.Errorf("created content = %q, want %q (UTF-8 BOM + header)", data, want)
	}
	// Idempotent: a second call is a no-op.
	if err := b.EnsureFile(); err != nil {
		t.Fatalf("second EnsureFile: %v", err)
	}
	data, err = os.ReadFile(p)
	if err != nil {
		t.Fatalf("re-reading created file: %v", err)
	}
	if string(data) != want {
		t.Errorf("content after second EnsureFile = %q, want unchanged %q", data, want)
	}
	// The header-only book loads cleanly with no names.
	if got := b.Lookup("144"); got != "" {
		t.Errorf("Lookup on header-only book = %q, want \"\"", got)
	}
}

func TestEnsureFileNeverModifiesExisting(t *testing.T) {
	p := writeContactsFile(t, t.TempDir(), "144,Alice\n")
	b := New(p)
	if err := b.EnsureFile(); err != nil {
		t.Fatalf("EnsureFile on existing file: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if string(data) != "144,Alice\n" {
		t.Errorf("existing content = %q, want it byte-for-byte unchanged", data)
	}
}

func TestEnsureFileDirectoryIsError(t *testing.T) {
	d := t.TempDir() // the path IS a directory
	b := New(d)
	err := b.EnsureFile()
	if err == nil {
		t.Fatal("EnsureFile on a directory: wanted an error, got none")
	}
	want := fmt.Sprintf("%s is not a regular file", d)
	if err.Error() != want {
		t.Fatalf("wanted %q, got %q", want, err.Error())
	}
}

func TestEnsureFileMkdirError(t *testing.T) {
	d := t.TempDir()
	blocker := filepath.Join(d, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing blocker: %v", err)
	}
	// The parent "directory" is a regular file: MkdirAll must fail.
	b := New(filepath.Join(blocker, "contacts.csv"))
	if err := b.EnsureFile(); err == nil {
		t.Fatal("EnsureFile with a file blocking the parent dir: wanted an error, got none")
	}
}

func TestEnsureFileOpenError(t *testing.T) {
	// A NUL in the filename fails Stat and OpenFile portably; the OpenFile
	// error is not IsExist, so it must be returned.
	b := New(filepath.Join(t.TempDir(), "bad\x00name.csv"))
	if err := b.EnsureFile(); err == nil {
		t.Fatal("EnsureFile with an invalid filename: wanted an error, got none")
	}
}

// --- Concurrency (go test -race) ---

func TestBookConcurrentReadersAndWriter(t *testing.T) {
	p := writeContactsFile(t, t.TempDir(), "144,Alice\n")
	b := New(p)

	const readers = 8
	var wg sync.WaitGroup
	for range readers {
		wg.Go(func() {
			for range 200 {
				b.Lookup("144")
				b.Display("144")
				b.Version()
			}
		})
	}
	wg.Go(func() {
		for i := range 60 {
			var err error
			switch i % 3 {
			case 0: // rewrite with varying content
				err = os.WriteFile(p, []byte(fmt.Sprintf("144,Name%d\n", i)), 0o644)
			case 1: // delete
				err = os.Remove(p)
				if os.IsNotExist(err) {
					err = nil
				} else if err != nil && runtime.GOOS == "windows" {
					// Defensive: Go's os.Open DOES pass FILE_SHARE_DELETE,
					// so our own concurrent readers do not block deletion —
					// but a third-party handle without share-delete (e.g. an
					// antivirus scanner) can still make Remove fail here.
					err = nil
				}
			case 2: // malformed rewrite
				err = os.WriteFile(p, []byte("144,\xFF\n"), 0o644)
			}
			if err != nil {
				t.Errorf("writer iteration %d: %v", i, err)
			}
		}
	})
	wg.Wait()

	// Settle to a known state (different size than the last malformed write,
	// so the signature is guaranteed to change).
	writeContactsFile(t, filepath.Dir(p), "144,Alice\n")
	if got := b.Lookup("144"); got != "Alice" {
		t.Errorf("Lookup after concurrent churn = %q, want %q", got, "Alice")
	}
}

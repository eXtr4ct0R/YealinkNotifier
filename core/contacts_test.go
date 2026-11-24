package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"YealinkNotifier/contacts"
)

// newBook writes a real CSV in a temp dir and returns a contacts.Book over
// it (no mocks — the book is a stdlib-only leaf package).
func newBook(t *testing.T, content string) *contacts.Book {
	t.Helper()
	p := filepath.Join(t.TempDir(), "contacts.csv")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", p, err)
	}
	return contacts.New(p)
}

func TestCallerMessage(t *testing.T) {
	book := newBook(t, "144,Alice\n")
	tests := []struct {
		name   string
		book   *contacts.Book
		caller string
		want   string
	}{
		{name: "known contact", book: book, caller: "144", want: "Caller: Alice (144)"},
		{name: "unknown number", book: book, caller: "999", want: "Caller number: 999"},
		{name: "empty caller with book", book: book, caller: "", want: "You are being called! Check your office phone."},
		{name: "nil book falls back to number", book: nil, caller: "144", want: "Caller number: 144"},
		{name: "nil book empty caller", book: nil, caller: "", want: "You are being called! Check your office phone."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(false, false)
			a.SetContacts(tt.book)
			if got := a.callerMessage(tt.caller); got != tt.want {
				t.Errorf("callerMessage(%q) = %q, want %q", tt.caller, got, tt.want)
			}
		})
	}
}

func TestCallerMessageMalformedBookDegradesToNumber(t *testing.T) {
	a := New(false, false)
	a.SetContacts(newBook(t, "144,\xFF\n")) // invalid UTF-8: fail-open, no names
	if got := a.callerMessage("144"); got != "Caller number: 144" {
		t.Errorf("callerMessage with malformed book = %q, want %q", got, "Caller number: 144")
	}
}

// The notification body must be safe for beeep's Windows toast path:
// go-toast interpolates it into unescaped XML CDATA (a "]]>" would break
// out) and a PowerShell here-string (a newline followed by a line starting
// "@ would terminate it). contacts.parse sanitizes names (control chars
// folded, XML-unsafe set stripped, 64-rune cap) and sanitizeCaller
// sanitizes numbers, so callerMessage output carries neither.
func TestCallerMessageBodyIsCDATASafe(t *testing.T) {
	a := New(false, false)
	a.SetContacts(newBook(t, "144,\"Evil]]>\n\"\"@echo pwned\"\n"))
	got := a.callerMessage("144")
	if want := "Caller: Evil]] @echo pwned (144)"; got != want {
		t.Errorf("callerMessage with hostile name = %q, want sanitized %q", got, want)
	}
	for _, bad := range []string{"]]>", "<", ">", "&", "\"", "'", "\n", "\r"} {
		if strings.Contains(got, bad) {
			t.Errorf("notification body contains %q: %q", bad, got)
		}
	}
}

func TestSetContactsAndContactsAccessor(t *testing.T) {
	a := New(false, false)
	if a.Contacts() != nil {
		t.Fatal("Contacts() before SetContacts must be nil")
	}
	b := newBook(t, "144,Alice\n")
	a.SetContacts(b)
	if a.Contacts() != b {
		t.Error("Contacts() must return the book installed by SetContacts")
	}
	a.SetContacts(nil)
	if a.Contacts() != nil {
		t.Error("SetContacts(nil) must clear the book")
	}
}

func TestOnCallWithBookRecordsRawNumber(t *testing.T) {
	a := New(false, false) // both channels off: record only, no notifications
	a.SetContacts(newBook(t, "144,Alice\n"))

	a.OnCall("sip:144@10.184.5.37_number")

	events := a.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	// The history stores the RAW sanitized number — names are display-only.
	if events[0].Caller != "144" {
		t.Errorf("Events()[0].Caller = %q, want raw %q (never \"Alice (144)\")", events[0].Caller, "144")
	}
	// The name is still resolvable for display at render time.
	if got := a.Contacts().Display(events[0].Caller); got != "Alice (144)" {
		t.Errorf("Display(recorded caller) = %q, want %q", got, "Alice (144)")
	}
}

func TestOnCallNilBookStillRecords(t *testing.T) {
	a := New(false, false) // no SetContacts: the nil book must be safe
	a.OnCall("144")
	a.OnCall("")

	events := a.Events()
	if len(events) != 2 {
		t.Fatalf("Events() len = %d, want 2", len(events))
	}
	if events[0].Caller != "unknown" {
		t.Errorf("Events()[0].Caller = %q, want %q", events[0].Caller, "unknown")
	}
	if events[1].Caller != "144" {
		t.Errorf("Events()[1].Caller = %q, want %q", events[1].Caller, "144")
	}
}

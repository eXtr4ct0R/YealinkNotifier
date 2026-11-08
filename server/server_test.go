package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"YealinkNotifier/core"
)

func TestParseAllowedIPs(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		wantNil bool
		wantErr bool
		allows  []string
		denies  []string
	}{
		{name: "nil input", in: nil, wantNil: true},
		{name: "empty input", in: []string{}, wantNil: true},
		{name: "blank entries only", in: []string{" ", "  "}, wantNil: true},
		{
			name:   "bare IPv4 becomes /32",
			in:     []string{"10.0.0.5"},
			allows: []string{"10.0.0.5"},
			denies: []string{"10.0.0.6"},
		},
		{
			name:   "bare IPv4 with padding",
			in:     []string{"  10.0.0.5  "},
			allows: []string{"10.0.0.5"},
			denies: []string{"10.0.0.6"},
		},
		{
			name:   "IPv4 CIDR",
			in:     []string{"192.168.1.0/24"},
			allows: []string{"192.168.1.1", "192.168.1.254", "::ffff:192.168.1.1"},
			denies: []string{"192.168.2.1", "10.0.0.1", "::ffff:192.168.2.1"},
		},
		{
			name:   "bare IPv6 becomes /128",
			in:     []string{"2001:db8::1"},
			allows: []string{"2001:db8::1"},
			denies: []string{"2001:db8::2"},
		},
		{
			name:   "IPv6 CIDR",
			in:     []string{"2001:db8::/32"},
			allows: []string{"2001:db8:ffff::1"},
			denies: []string{"2002::1", "192.168.1.1"},
		},
		{
			name:   "mixed list with blanks",
			in:     []string{"  10.0.0.5", "", "192.168.1.0/24", "2001:db8::1"},
			allows: []string{"10.0.0.5", "192.168.1.254", "2001:db8::1"},
			denies: []string{"10.0.0.6", "2001:db8::2"},
		},
		{name: "invalid entry", in: []string{"10.0.0.5", "nope"}, wantErr: true},
		{name: "invalid CIDR mask", in: []string{"10.0.0.0/33"}, wantErr: true},
		{name: "invalid CIDR mask v6", in: []string{"2001:db8::/129"}, wantErr: true},
		{name: "out-of-range address", in: []string{"300.1.2.3"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nets, err := parseAllowedIPs(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseAllowedIPs(%v) = %v, want error", tt.in, nets)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAllowedIPs(%v): %v", tt.in, err)
			}
			if tt.wantNil {
				if nets != nil {
					t.Fatalf("parseAllowedIPs(%v) = %v, want nil", tt.in, nets)
				}
				return
			}
			for _, ip := range tt.allows {
				if !ipAllowed(nets, mustParseIP(t, ip)) {
					t.Errorf("ipAllowed: %s should be allowed", ip)
				}
			}
			for _, ip := range tt.denies {
				if ipAllowed(nets, mustParseIP(t, ip)) {
					t.Errorf("ipAllowed: %s should be rejected", ip)
				}
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{remoteAddr: "10.0.0.5:43123", want: "10.0.0.5"},
		{remoteAddr: "10.0.0.5", want: "10.0.0.5"},
		{remoteAddr: "[::ffff:192.168.1.20]:43123", want: "192.168.1.20"},
		{remoteAddr: "[2001:db8::1]:43123", want: "2001:db8::1"},
		{remoteAddr: "not-an-ip:43123", want: ""},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = tt.remoteAddr
		var want net.IP
		if tt.want != "" {
			want = mustParseIP(t, tt.want)
		}
		if got := clientIP(r); !got.Equal(want) {
			t.Errorf("clientIP with RemoteAddr %q = %v, want %v", tt.remoteAddr, got, want)
		}
	}
}

func TestHandlerRejectsDisallowedSource(t *testing.T) {
	app := core.New(false, false)

	s, err := New(app, "127.0.0.1:0", []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/?number=100", nil)
	r.RemoteAddr = "8.8.8.8:43123"
	rr := httptest.NewRecorder()
	s.handler(rr, r)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if body := rr.Body.String(); body != "Forbidden\n" {
		t.Errorf("body = %q, want %q", body, "Forbidden\n")
	}
	if got := app.Events(); len(got) != 0 {
		t.Errorf("rejected request recorded %d event(s), want 0", len(got))
	}
}

func TestHandlerAllowsConfiguredSource(t *testing.T) {
	app := core.New(false, false) // don't send real notifications/beeps in tests

	s, err := New(app, "127.0.0.1:0", []string{"192.168.1.0/24"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/?number=100", nil)
	r.RemoteAddr = "192.168.1.20:43123"
	rr := httptest.NewRecorder()
	s.handler(rr, r)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); body != "OK" {
		t.Errorf("body = %q, want %q", body, "OK")
	}
}

func TestHandlerNoAllowListKeepsOldBehavior(t *testing.T) {
	app := core.New(false, false) // don't send real notifications/beeps in tests

	s, err := New(app, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/?number=100", nil)
	r.RemoteAddr = "8.8.8.8:43123"
	rr := httptest.NewRecorder()
	s.handler(rr, r)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); body != "OK" {
		t.Errorf("body = %q, want %q", body, "OK")
	}
}

func TestNewRejectsInvalidAllowedIPs(t *testing.T) {
	app := core.New(false, false)
	s, err := New(app, "127.0.0.1:0", []string{"10.0.0.5", "nonsense"})
	if err == nil {
		t.Fatalf("New with an invalid allowed-IP entry = %v, want error", s)
	}
	if s != nil {
		t.Errorf("New with an invalid allowed-IP entry = %v, want nil", s)
	}
}

func TestHandlerRejectsUnparseableClientIP(t *testing.T) {
	app := core.New(false, false)

	s, err := New(app, "127.0.0.1:0", []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/?number=100", nil)
	r.RemoteAddr = "not-an-ip:43123"
	rr := httptest.NewRecorder()
	s.handler(rr, r)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if got := app.Events(); len(got) != 0 {
		t.Errorf("rejected request recorded %d event(s), want 0", len(got))
	}
}

func TestHandlerRecordsNumber(t *testing.T) {
	app := core.New(false, false) // don't send real notifications/beeps in tests

	s, err := New(app, "127.0.0.1:0", []string{"192.168.1.0/24"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/?number=228", nil)
	r.RemoteAddr = "192.168.1.20:43123"
	rr := httptest.NewRecorder()
	s.handler(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	events := app.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	if events[0].Caller != "228" {
		t.Errorf("Events()[0].Caller = %q, want %q", events[0].Caller, "228")
	}
}

func TestHandlerRecordsCallerFallback(t *testing.T) {
	app := core.New(false, false) // don't send real notifications/beeps in tests

	s, err := New(app, "127.0.0.1:0", []string{"192.168.1.0/24"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/?caller=333", nil)
	r.RemoteAddr = "192.168.1.20:43123"
	rr := httptest.NewRecorder()
	s.handler(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	events := app.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	if events[0].Caller != "333" {
		t.Errorf("Events()[0].Caller = %q, want %q", events[0].Caller, "333")
	}
}

func TestHandlerNumberWinsOverCaller(t *testing.T) {
	app := core.New(false, false) // don't send real notifications/beeps in tests

	s, err := New(app, "127.0.0.1:0", []string{"192.168.1.0/24"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/?number=228&caller=333", nil)
	r.RemoteAddr = "192.168.1.20:43123"
	rr := httptest.NewRecorder()
	s.handler(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	events := app.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	if events[0].Caller != "228" {
		t.Errorf("Events()[0].Caller = %q, want %q (%q must win)", events[0].Caller, "228", "number")
	}
}

// mustParseIP parses ip and fails the test if it is not a valid IP
// (all test fixtures must be valid).
func mustParseIP(t *testing.T, ip string) net.IP {
	t.Helper()
	v := net.ParseIP(ip)
	if v == nil {
		t.Fatalf("test fixture %q is not a valid IP", ip)
	}
	return v
}

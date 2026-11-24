package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", p, err)
	}
	return p
}

func pointerOf[T any](v T) *T { return &v }

// chdirIsolated points the CWD and the user config dir at fresh empty
// locations under a temp dir (hermetic, per-OS env names like
// contacts_test.go). HOME is set too: on macOS os.UserConfigDir ignores
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

func assertAllPointersNil(t *testing.T, f file) {
	t.Helper()
	if f.Port != nil || f.Addr != nil || f.Notify != nil || f.Beep != nil ||
		f.Logfile != nil || f.Quiet != nil || f.AllowedIPs != nil || f.Contacts != nil {
		t.Fatalf("expected all-nil / empty file view, got %+v", f)
	}
}

// --- Schema & defaults ---

func TestDecodeStrictEmptyFileIsAllDefaults(t *testing.T) {
	f, err := decodeStrict(nil, "config.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertAllPointersNil(t, f)
}

func TestDecodeStrictEmptyMappingIsAllDefaults(t *testing.T) {
	f, err := decodeStrict([]byte("{}\n"), "config.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertAllPointersNil(t, f)
}

func TestDecodeStrictCommentsAndWhitespaceOnly(t *testing.T) {
	f, err := decodeStrict([]byte("# just a comment\n\n   \n\t\n# another\n"), "config.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertAllPointersNil(t, f)
}

func TestDecodeStrictReadsEveryValidKey(t *testing.T) {
	doc := "port: 0\n" +
		"addr: 127.0.0.1\n" +
		"notify: false\n" +
		"beep: false\n" +
		"allowed_ips: [\"10.0.0.1\", \"192.168.1.0/24\"]\n" +
		"logfile: \"\"\n" +
		"quiet: true\n" +
		"contacts: \"book.csv\"\n"
	f, err := decodeStrict([]byte(doc), "config.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Zero values must be PRESENT (non-nil) values, not "absent".
	if f.Port == nil || *f.Port != 0 {
		t.Errorf("port: wanted present 0, got %v", f.Port)
	}
	if f.Addr == nil || *f.Addr != "127.0.0.1" {
		t.Errorf("addr: wanted present 127.0.0.1, got %v", f.Addr)
	}
	if f.Notify == nil || *f.Notify != false {
		t.Errorf("notify: wanted present false, got %v", f.Notify)
	}
	if f.Beep == nil || *f.Beep != false {
		t.Errorf("beep: wanted present false, got %v", f.Beep)
	}
	if f.Logfile == nil || *f.Logfile != "" {
		t.Errorf("logfile: wanted present empty string, got %v", f.Logfile)
	}
	if f.Quiet == nil || *f.Quiet != true {
		t.Errorf("quiet: wanted present true, got %v", f.Quiet)
	}
	want := []string{"10.0.0.1", "192.168.1.0/24"}
	if !slices.Equal(f.AllowedIPs, want) {
		t.Errorf("allowed_ips: wanted %v, got %v", want, f.AllowedIPs)
	}
	if f.Contacts == nil || *f.Contacts != "book.csv" {
		t.Errorf("contacts: wanted present book.csv, got %v", f.Contacts)
	}
}

func TestDecodeStrictContactsPresentEmpty(t *testing.T) {
	f, err := decodeStrict([]byte("contacts: \"\"\n"), "config.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A present empty string is a VALUE (auto-discover), not "absent".
	if f.Contacts == nil || *f.Contacts != "" {
		t.Errorf("contacts: wanted present empty string, got %v", f.Contacts)
	}
}

func TestMergeUnsetKeyFallsBackToDefault(t *testing.T) {
	eff := merge(file{}, Flags{})
	if eff.Port != DefaultPort {
		t.Errorf("Port: wanted default %d, got %d", DefaultPort, eff.Port)
	}
	if eff.Addr != DefaultAddr {
		t.Errorf("Addr: wanted default %q, got %q", DefaultAddr, eff.Addr)
	}
	if eff.Notify != DefaultNotify {
		t.Errorf("Notify: wanted default %v, got %v", DefaultNotify, eff.Notify)
	}
	if eff.Beep != DefaultBeep {
		t.Errorf("Beep: wanted default %v, got %v", DefaultBeep, eff.Beep)
	}
	if eff.Logfile != DefaultLogfile() {
		t.Errorf("Logfile: wanted default %q, got %q", DefaultLogfile(), eff.Logfile)
	}
	if eff.Quiet != DefaultQuiet {
		t.Errorf("Quiet: wanted default %v, got %v", DefaultQuiet, eff.Quiet)
	}
	if eff.AllowedIPs != nil {
		t.Errorf("AllowedIPs: wanted nil (allow all), got %v", eff.AllowedIPs)
	}
	if eff.Contacts != DefaultContacts {
		t.Errorf("Contacts: wanted default %q, got %q", DefaultContacts, eff.Contacts)
	}
}

// --- Resolution ---

func TestSearchCandidatesOrderAndContents(t *testing.T) {
	t.Run("orderAndContents", func(t *testing.T) {
		var want []string
		if exe, err := os.Executable(); err == nil {
			want = append(want, filepath.Join(filepath.Dir(exe), "config.yml"))
		}
		if wd, err := os.Getwd(); err == nil {
			want = append(want, filepath.Join(wd, "config.yml"))
		}
		if dir, err := os.UserConfigDir(); err == nil {
			want = append(want, filepath.Join(dir, "config.yml"))
		}
		got := searchCandidates()
		if !slices.Equal(got, want) {
			t.Fatalf("candidates:\n got  %v\n want %v", got, want)
		}
	})

	t.Run("errorBaseIsSkipped", func(t *testing.T) {
		// Break os.Getwd: chdir into a directory we then delete.
		base := t.TempDir()
		sub := filepath.Join(base, "cwd")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		t.Chdir(sub)
		if err := os.Remove(sub); err != nil {
			// Windows refuses to remove a directory while it is a process CWD.
			t.Skipf("cannot remove a live CWD on this OS: %v", err)
		}
		t.Chdir(base)
		_, wdErr := os.Getwd()
		if wdErr == nil {
			t.Skip("deleting the CWD does not make Getwd fail on this OS")
		}
		got := searchCandidates()
		for _, c := range got {
			if filepath.Dir(c) == sub {
				t.Fatalf("candidate %q must be skipped: its base errors", c)
			}
		}
	})
}

func TestFirstExistingRegularFileWins(t *testing.T) {
	d := t.TempDir()
	missing := filepath.Join(d, "no-such-dir", "config.yml")
	dir := filepath.Join(d, "dir-config.yml")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	regular := writeConfig(t, d, "file-config.yml", "port: 8080\n")

	got := firstExistingRegularFile([]string{missing, dir, regular})
	if got != regular {
		t.Fatalf("wanted the regular file %q, got %q", regular, got)
	}
	if got := firstExistingRegularFile([]string{missing, dir}); got != "" {
		t.Fatalf("no candidate is an existing regular file: wanted \"\", got %q", got)
	}
}

func TestLoadSearchUsesCWDThenUserConfigDir(t *testing.T) {
	cwd, ucfg := chdirIsolated(t)
	writeConfig(t, ucfg, "config.yml", "port: 1111\n")

	cwdFile := writeConfig(t, cwd, "config.yml", "port: 2222\n")
	eff, path, err := Load("", Flags{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := cwdFile
	if abs, aerr := filepath.Abs(want); aerr == nil {
		want = abs
	}
	if path != want {
		t.Fatalf("wanted the CWD config file %q, got %q", want, path)
	}
	if eff.Port != 2222 {
		t.Errorf("Port: wanted 2222 (from the CWD file), got %d", eff.Port)
	}
}

func TestLoadSearchSkipsDirectoryCandidate(t *testing.T) {
	cwd, ucfg := chdirIsolated(t)
	// A DIRECTORY named config.yml in the CWD: present, but not a regular file.
	if err := os.Mkdir(filepath.Join(cwd, "config.yml"), 0o755); err != nil {
		t.Fatalf("mkdir dir-candidate: %v", err)
	}
	ucfgFile := writeConfig(t, ucfg, "config.yml", "port: 3333\n")

	eff, path, err := Load("", Flags{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := ucfgFile
	if abs, aerr := filepath.Abs(want); aerr == nil {
		want = abs
	}
	if path != want {
		t.Fatalf("directory candidate must be skipped, wanted %q, got %q", want, path)
	}
	if eff.Port != 3333 {
		t.Errorf("Port: wanted 3333 (from the user config dir file), got %d", eff.Port)
	}
}

func TestLoadExplicitModeUsesExactPathAndReturnsAbsolute(t *testing.T) {
	d := t.TempDir()
	sub := filepath.Join(d, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	exact := writeConfig(t, sub, "my-config.yml", "port: 4444\n")
	t.Chdir(d)

	eff, path, err := Load("sub/my-config.yml", Flags{}) // RELATIVE path on input
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := exact
	if abs, aerr := filepath.Abs(want); aerr == nil {
		want = abs
	}
	if path != want {
		t.Fatalf("wanted the absolute path %q, got %q", want, path)
	}
	if eff.Port != 4444 {
		t.Errorf("Port: wanted 4444, got %d", eff.Port)
	}
}

// --- Precedence ---

func TestPrecedenceFlagOverConfig(t *testing.T) {
	f := file{
		Port:   pointerOf(9000),
		Addr:   pointerOf("10.0.0.1"),
		Notify: pointerOf(false),
	}
	eff := merge(f, Flags{
		Port: 7000, PortSet: true,
		Addr: "127.0.0.1", AddrSet: true,
		Notify: true, NotifySet: true,
	})
	if eff.Port != 7000 {
		t.Errorf("Port: wanted flag 7000, got %d", eff.Port)
	}
	if eff.Addr != "127.0.0.1" {
		t.Errorf("Addr: wanted flag 127.0.0.1, got %q", eff.Addr)
	}
	if eff.Notify != true {
		t.Errorf("Notify: wanted flag true, got %v", eff.Notify)
	}
	// Unset flag, absent in file: default.
	if eff.Beep != DefaultBeep {
		t.Errorf("Beep: wanted default %v, got %v", DefaultBeep, eff.Beep)
	}
}

func TestPrecedenceFlagDefaultIsExplicit(t *testing.T) {
	// A flag EXPLICITLY set to its default value (8080) still beats the file.
	eff := merge(file{Port: pointerOf(9000)}, Flags{Port: 8080, PortSet: true})
	if eff.Port != 8080 {
		t.Fatalf("Port: wanted explicit flag 8080, got %d", eff.Port)
	}
}

func TestPrecedenceConfigOverDefault(t *testing.T) {
	f := file{
		Port:   pointerOf(9000),
		Notify: pointerOf(false),
	}
	eff := merge(f, Flags{})
	if eff.Port != 9000 {
		t.Errorf("Port: wanted file 9000, got %d", eff.Port)
	}
	if eff.Notify != false {
		t.Errorf("Notify: wanted file false, got %v", eff.Notify)
	}
	if eff.Beep != DefaultBeep {
		t.Errorf("Beep: wanted default %v, got %v", DefaultBeep, eff.Beep)
	}
}

func TestPrecedencePerParameterIndependent(t *testing.T) {
	f := file{Notify: pointerOf(false)} // only Notify comes from the file
	eff := merge(f, Flags{
		Port: 6161, PortSet: true, // Port from the flag
		Beep:    true,           // value set, PRESENCE not set → ignored
		Logfile: "/ignored.log", // value set, PRESENCE not set → ignored
	})
	if eff.Port != 6161 {
		t.Errorf("Port: wanted flag 6161, got %d", eff.Port)
	}
	if eff.Notify != false {
		t.Errorf("Notify: wanted file false, got %v", eff.Notify)
	}
	if eff.Beep != DefaultBeep {
		t.Errorf("Beep: wanted default %v (flag not set), got %v", DefaultBeep, eff.Beep)
	}
	if eff.Logfile != DefaultLogfile() {
		t.Errorf("Logfile: wanted default, got %q", eff.Logfile)
	}
}

func TestPrecedenceContacts(t *testing.T) {
	// YAML value beats the default "".
	eff := merge(file{Contacts: pointerOf("book.csv")}, Flags{})
	if eff.Contacts != "book.csv" {
		t.Errorf("Contacts: wanted file book.csv, got %q", eff.Contacts)
	}
	// An explicitly EMPTY flag beats the YAML value (re-enables discovery).
	eff = merge(file{Contacts: pointerOf("book.csv")}, Flags{Contacts: "", ContactsSet: true})
	if eff.Contacts != "" {
		t.Errorf("Contacts: wanted explicit-empty flag \"\", got %q", eff.Contacts)
	}
	// A non-empty flag value beats the YAML value.
	eff = merge(file{Contacts: pointerOf("book.csv")}, Flags{Contacts: "flag.csv", ContactsSet: true})
	if eff.Contacts != "flag.csv" {
		t.Errorf("Contacts: wanted flag flag.csv, got %q", eff.Contacts)
	}
	// Value set but PRESENCE not set → ignored.
	eff = merge(file{Contacts: pointerOf("book.csv")}, Flags{Contacts: "ignored.csv"})
	if eff.Contacts != "book.csv" {
		t.Errorf("Contacts: wanted file book.csv (flag not set), got %q", eff.Contacts)
	}
	// Unset everywhere → default.
	eff = merge(file{}, Flags{})
	if eff.Contacts != DefaultContacts {
		t.Errorf("Contacts: wanted default %q, got %q", DefaultContacts, eff.Contacts)
	}
}

// --- Errors (exact message formats) ---

func TestE1UnparseableYAMLMessages(t *testing.T) {
	const path = "some/dir/config.yml"
	cases := []string{
		"5\n",          // bare scalar root
		"[1, 2]\n",     // bare sequence root
		"port: [1,\n",  // unparseable YAML
		"a: b\na: c\n", // duplicate key
	}
	for _, doc := range cases {
		_, err := decodeStrict([]byte(doc), path)
		if err == nil {
			t.Errorf("doc %q: wanted an E1 error, got none", doc)
			continue
		}
		prefix := fmt.Sprintf("config error: failed to parse %s: ", path)
		if len(err.Error()) <= len(prefix) || err.Error()[:len(prefix)] != prefix {
			t.Errorf("doc %q: wanted message starting %q, got %q", doc, prefix, err.Error())
		}
	}
}

func TestE2UnknownKeyDeterministic(t *testing.T) {
	// Two unknown keys: alphabetically first ("alpha") must be reported.
	_, err := decodeStrict([]byte("zeta: 1\nalpha: 2\n"), "cfg.yml")
	if err == nil {
		t.Fatal("wanted an E2 error, got none")
	}
	want := `config error: unknown key "alpha" in cfg.yml`
	if err.Error() != want {
		t.Fatalf("wanted %q, got %q", want, err.Error())
	}
}

func TestE3WrongTypePerKey(t *testing.T) {
	const path = "cfg.yml"
	cases := []struct {
		doc      string
		expected string
		got      string
	}{
		{`port: "8080"`, "int", "string"},
		{`addr: 12345`, "string", "int"},
		{`notify: 1`, "bool", "int"},
		{`beep: "true"`, "bool", "string"},
		{`allowed_ips: "10.0.0.1"`, "list", "string"},
		{`allowed_ips: 10`, "list", "int"},
		{`logfile: 7`, "string", "int"},
		{`quiet: "no"`, "bool", "string"},
		{`contacts: 123`, "string", "int"},
	}
	for _, tc := range cases {
		_, err := decodeStrict([]byte(tc.doc), path)
		if err == nil {
			t.Errorf("doc %q: wanted an E3 error, got none", tc.doc)
			continue
		}
		key := tc.doc[:strings.IndexByte(tc.doc, ':')]
		want := fmt.Sprintf(`config error: key %q in %s has wrong type (expected %s, got %s)`,
			key, path, tc.expected, tc.got)
		if err.Error() != want {
			t.Errorf("doc %q:\n wanted %q\n got    %q", tc.doc, want, err.Error())
		}
	}
}

func TestE7ExplicitMissingFile(t *testing.T) {
	raw := "definitely/missing/config.yml"
	_, _, err := Load(raw, Flags{})
	if err == nil {
		t.Fatal("wanted an E7 error, got none")
	}
	want := "config error: config file not found: " + raw
	if err.Error() != want {
		t.Fatalf("wanted %q, got %q", want, err.Error())
	}
}

func TestE8ExplicitIsDirectory(t *testing.T) {
	d := t.TempDir()
	_, _, err := Load(d, Flags{})
	if err == nil {
		t.Fatal("wanted an E8 error, got none")
	}
	want := "config error: " + d + " is not a regular file"
	if err.Error() != want {
		t.Fatalf("wanted %q, got %q", want, err.Error())
	}
}

func TestE9FileUnreadable(t *testing.T) {
	p := writeConfig(t, t.TempDir(), "config.yml", "port: 8080\n")
	if runtime.GOOS == "windows" {
		// chmod 000 does not reliably deny reads on Windows — instead just
		// verify a valid readable file loads (no false failure).
		if _, _, err := Load(p, Flags{}); err != nil {
			t.Fatalf("readable config file should load: %v", err)
		}
		t.Skip("read denial not enforceable on Windows")
	}
	if err := os.Chmod(p, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(p, 0o644) })
	_, _, err := Load(p, Flags{})
	if err == nil {
		t.Fatal("wanted an E9 error, got none")
	}
	prefix := "config error: failed to read "
	if len(err.Error()) <= len(prefix) || err.Error()[:len(prefix)] != prefix {
		t.Fatalf("wanted message starting %q, got %q", prefix, err.Error())
	}
}

func TestAllowedIPsFromConfigPassedThroughUnmodified(t *testing.T) {
	// Normalization (trim / drop empties) is main's job, not config's.
	f, err := decodeStrict([]byte(`allowed_ips: [" 10.0.0.1 ", "192.168.1.0/24 "]`), "cfg.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	eff := merge(f, Flags{})
	want := []string{" 10.0.0.1 ", "192.168.1.0/24 "}
	if !slices.Equal(eff.AllowedIPs, want) {
		t.Fatalf("AllowedIPs: wanted raw pass-through %v, got %v", want, eff.AllowedIPs)
	}
}

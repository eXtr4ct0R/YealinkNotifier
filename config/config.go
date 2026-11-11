package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPort   = 8080
	DefaultAddr   = "0.0.0.0"
	DefaultNotify = true
	DefaultBeep   = true
	DefaultQuiet  = false
)

func DefaultLogfile() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "YealinkNotifier", "yealinknotifier.log")
}

type Effective struct {
	Port       int
	Addr       string
	Notify     bool
	Beep       bool
	AllowedIPs []string // nil/empty = allow all
	Logfile    string   // "" = console
	Quiet      bool
}

type Flags struct {
	Port          int
	Addr          string
	Notify        bool
	Beep          bool
	AllowedIPs    []string
	Logfile       string
	Quiet         bool
	PortSet       bool
	AddrSet       bool
	NotifySet     bool
	BeepSet       bool
	AllowedIPsSet bool
	LogfileSet    bool
	QuietSet      bool
}

type file struct {
	Port       *int     `yaml:"port"`
	Addr       *string  `yaml:"addr"`
	Notify     *bool    `yaml:"notify"`
	Beep       *bool    `yaml:"beep"`
	AllowedIPs []string `yaml:"allowed_ips"`
	Logfile    *string  `yaml:"logfile"`
	Quiet      *bool    `yaml:"quiet"`
}

var schema = map[string]string{
	"port":        "int",
	"addr":        "string",
	"notify":      "bool",
	"beep":        "bool",
	"allowed_ips": "list",
	"logfile":     "string",
	"quiet":       "bool",
}

func Load(configPath string, f Flags) (Effective, string, error) {
	var path string
	if configPath != "" {
		// Explicit mode: the exact path the user gave.
		st, err := os.Stat(configPath)
		if err != nil {
			return Effective{}, "", fmt.Errorf("config error: config file not found: %s", configPath)
		}
		if !st.Mode().IsRegular() {
			return Effective{}, "", fmt.Errorf("config error: %s is not a regular file", configPath)
		}
		path, err = filepath.Abs(configPath)
		if err != nil {
			path = configPath
		}
	} else {
		// Search mode: next to the binary, in the CWD, in the user config dir.
		path = firstExistingRegularFile(searchCandidates())
		if path == "" {
			return merge(file{}, f), "", nil
		}
	}

	data, err := readConfigFile(path)
	if err != nil {
		return Effective{}, "", fmt.Errorf("config error: failed to read %s: %v", path, err)
	}
	cfg, err := decodeStrict(data, path)
	if err != nil {
		return Effective{}, "", err
	}
	return merge(cfg, f), path, nil
}

func searchCandidates() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "config.yml"))
	}
	if wd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(wd, "config.yml"))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, "config.yml"))
	}
	return out
}

func firstExistingRegularFile(candidates []string) string {
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() {
			return c
		}
	}
	return ""
}

func readConfigFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func decodeStrict(data []byte, absPath string) (file, error) {
	var parsed map[string]yaml.Node
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return file{}, fmt.Errorf("config error: failed to parse %s: %v", absPath, err)
	}

	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		expected, known := schema[k]
		if !known {
			return file{}, fmt.Errorf("config error: unknown key %q in %s", k, absPath)
		}
		got := typeName(parsed[k].Tag)
		if got != expected {
			return file{}, fmt.Errorf("config error: key %q in %s has wrong type (expected %s, got %s)", k, absPath, expected, got)
		}
	}

	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return file{}, fmt.Errorf("config error: failed to parse %s: %v", absPath, err)
	}
	return f, nil
}

func typeName(tag string) string {
	switch tag {
	case "!!int":
		return "int"
	case "!!str":
		return "string"
	case "!!bool":
		return "bool"
	case "!!seq":
		return "list"
	case "!!map":
		return "map"
	case "!!float":
		return "float"
	case "!!null":
		return "null"
	default:
		return tag
	}
}

func merge(f file, flags Flags) Effective {
	eff := Effective{
		Port:       DefaultPort,
		Addr:       DefaultAddr,
		Notify:     DefaultNotify,
		Beep:       DefaultBeep,
		Logfile:    DefaultLogfile(),
		Quiet:      DefaultQuiet,
		AllowedIPs: f.AllowedIPs, // nil OK = allow all
	}
	if f.Port != nil {
		eff.Port = *f.Port
	}
	if f.Addr != nil {
		eff.Addr = *f.Addr
	}
	if f.Notify != nil {
		eff.Notify = *f.Notify
	}
	if f.Beep != nil {
		eff.Beep = *f.Beep
	}
	if f.Logfile != nil {
		eff.Logfile = *f.Logfile
	}
	if f.Quiet != nil {
		eff.Quiet = *f.Quiet
	}

	if flags.PortSet {
		eff.Port = flags.Port
	}
	if flags.AddrSet {
		eff.Addr = flags.Addr
	}
	if flags.NotifySet {
		eff.Notify = flags.Notify
	}
	if flags.BeepSet {
		eff.Beep = flags.Beep
	}
	if flags.AllowedIPsSet {
		eff.AllowedIPs = flags.AllowedIPs
	}
	if flags.LogfileSet {
		eff.Logfile = flags.Logfile
	}
	if flags.QuietSet {
		eff.Quiet = flags.Quiet
	}
	return eff
}

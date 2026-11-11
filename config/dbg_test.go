package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDbgTmp(t *testing.T) {
	data := []byte("allowed_ips: [\" 10.0.0.1 \", \"192.168.1.0/24 \"]\n")
	var x map[string]yaml.Node
	if e := yaml.Unmarshal(data, &x); e != nil {
		t.Fatal(e)
	}
	var f file
	if e := yaml.Unmarshal(data, &f); e != nil {
		t.Fatal(e)
	}
	t.Logf("OK: %v", f.AllowedIPs)
}

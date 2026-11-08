package core

import (
	"strings"
	"testing"
)

func TestSanitizeCaller(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "SIP URI with Yealink suffix", in: "sip:144@10.184.5.37_number", want: "144"},
		{name: "SIP URI", in: "sip:144@10.184.5.37", want: "144"},
		{name: "SIPS URI with Yealink suffix", in: "sips:+79991234567@pbx.example.com_number", want: "+79991234567"},
		{name: "SIP URI with parameters", in: "sip:100;tag=abc@10.0.0.1_number", want: "100"},
		{name: "SIP URI with suffix in user part", in: "sip:144_number@10.184.5.37", want: "144"},
		{name: "uppercase scheme", in: "SIP:144@10.184.5.37", want: "144"},
		{name: "suffix only, strips to empty", in: "_number", want: ""},
		{name: "plain number", in: "144", want: "144"},
		{name: "plain number with Yealink suffix", in: "144_number", want: "144"},
		{name: "empty", in: "", want: ""},
		{name: "padded with spaces", in: "  144  ", want: "144"},
		{name: "formatted number untouched", in: "+1-800-555-0199", want: "+1-800-555-0199"},
		{name: "digit-only suffix is kept (suffix must start with a letter)", in: "job_1234", want: "job_1234"},
		{name: "XML unsafe chars stripped", in: "abc<&>'\"", want: "abc"},
		{
			name: "truncated to 64 runes",
			in:   strings.Repeat("a", 70),
			want: strings.Repeat("a", 64),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeCaller(tt.in); got != tt.want {
				t.Errorf("sanitizeCaller(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

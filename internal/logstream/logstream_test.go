package logstream

import "testing"

func TestStripANSI(t *testing.T) {
	cases := map[string]string{
		"\x1b[32m✓ built\x1b[39m":     "✓ built",
		"progress 10%\rprogress 100%": "progress 100%",
		"plain":                       "plain",
	}
	for in, want := range cases {
		if got := stripANSI(in); got != want {
			t.Errorf("stripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

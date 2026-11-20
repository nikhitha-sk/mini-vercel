package api

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My Portfolio!":     "my-portfolio",
		"  vite--demo  ":    "vite-demo",
		"Ünïcode & Friends": "n-code-friends",
		"":                  "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanDir(t *testing.T) {
	for in, want := range map[string]string{"": ".", "./": ".", "dist": "dist", "/build/": "build", `apps\web`: "apps/web"} {
		if got, err := cleanDir(in); err != nil || got != want {
			t.Errorf("cleanDir(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := cleanDir("../secrets"); err == nil {
		t.Error("expected traversal to be rejected")
	}
}

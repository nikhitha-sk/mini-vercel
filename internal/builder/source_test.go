package builder

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestExtractZipSingleTopFolderBecomesRoot(t *testing.T) {
	dir := t.TempDir()
	root, err := ExtractZip(makeZip(t, map[string]string{"site-main/index.html": "hi", "site-main/a/b.css": "x"}), dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) != "site-main" {
		t.Fatalf("root = %s", root)
	}
	if _, err := os.Stat(filepath.Join(root, "a", "b.css")); err != nil {
		t.Fatal(err)
	}
}

func TestExtractZipNormalisesBackslashes(t *testing.T) {
	dir := t.TempDir()
	root, err := ExtractZip(makeZip(t, map[string]string{"index.html": "hi", `src\main.js`: "x"}), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "main.js")); err != nil {
		t.Fatalf("backslash path not normalised: %v", err)
	}
}

func TestExtractZipRejectsZipSlip(t *testing.T) {
	_, err := ExtractZip(makeZip(t, map[string]string{"../../evil.txt": "x"}), t.TempDir())
	if !IsUserError(err) {
		t.Fatalf("expected user error, got %v", err)
	}
}

func TestValidateRepoURL(t *testing.T) {
	ok := map[string]string{
		"https://github.com/vitejs/vite":    "https://github.com/vitejs/vite",
		"github.com/vitejs/vite/":           "https://github.com/vitejs/vite",
		"https://gitlab.com/group/sub/repo": "https://gitlab.com/group/sub/repo",
	}
	for in, want := range ok {
		got, err := ValidateRepoURL(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "file:///etc/passwd", "ssh://git@github.com/a/b", "https://user:pw@github.com/a/b", "https://github.com/onlyowner", "http://github.com/a/b"} {
		if _, err := ValidateRepoURL(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

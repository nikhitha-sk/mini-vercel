package builder

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectStaticSite(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"index.html": "<h1>hi</h1>"})
	d := Detect(dir)
	if d.Framework != "static" || d.HasPackageJSON || d.Build != "" || d.OutputDirs[0] != "." {
		t.Fatalf("unexpected: %+v", d)
	}
}

func TestDetectStaticInPublic(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"public/index.html": "x", "README.md": "y"})
	if d := Detect(dir); d.OutputDirs[0] != "public" {
		t.Fatalf("got %v", d.OutputDirs)
	}
}

func TestDetectViteWithPnpm(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"package.json":   `{"scripts":{"build":"vite build"},"devDependencies":{"vite":"^6"},"engines":{"node":">=20"}}`,
		"pnpm-lock.yaml": "lockfileVersion: '9.0'",
	})
	d := Detect(dir)
	if d.Framework != "vite" || d.PackageManager != "pnpm" || d.Build != "npm run build" || d.OutputDirs[0] != "dist" || d.NodeMajor != 20 {
		t.Fatalf("unexpected: %+v", d)
	}
}

func TestDetectNextWithoutExportWarns(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"package.json":      `{"scripts":{"build":"next build"},"dependencies":{"next":"15","react":"19"}}`,
		"package-lock.json": "{}",
		"next.config.js":    "module.exports = {}",
	})
	d := Detect(dir)
	if d.Framework != "nextjs" || d.Install != "npm ci || npm install" || len(d.Warnings) == 0 {
		t.Fatalf("unexpected: %+v", d)
	}
	write(t, dir, map[string]string{"next.config.js": "module.exports = { output: 'export' }"})
	if d := Detect(dir); len(d.Warnings) != 0 {
		t.Fatalf("static export should not warn: %v", d.Warnings)
	}
}

func TestDetectCRAAndNvmrc(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"package.json": `{"scripts":{"build":"react-scripts build"},"dependencies":{"react-scripts":"5"}}`,
		"yarn.lock":    "",
		".nvmrc":       "v18.19.0",
	})
	d := Detect(dir)
	if d.Framework != "cra" || d.PackageManager != "yarn" || d.OutputDirs[0] != "build" || d.NodeMajor != 18 {
		t.Fatalf("unexpected: %+v", d)
	}
}

func TestDetectNoBuildScript(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"package.json": `{"name":"x"}`, "index.html": "x"})
	d := Detect(dir)
	if d.Build != "" || !reflect.DeepEqual(d.OutputDirs[:2], []string{"public", "."}) {
		t.Fatalf("unexpected: %+v", d)
	}
}

func TestPnpmVersionFromLockfile(t *testing.T) {
	for lock, want := range map[string]string{
		"lockfileVersion: 5.3\n":   "corepack pnpm@6",
		"lockfileVersion: 5.4\n":   "corepack pnpm@7",
		"lockfileVersion: '6.0'\n": "corepack pnpm@8",
		"lockfileVersion: '9.0'\n": "corepack pnpm@9",
	} {
		dir := t.TempDir()
		write(t, dir, map[string]string{"package.json": `{"devDependencies":{"vite":"6"},"scripts":{"build":"vite build"}}`, "pnpm-lock.yaml": lock})
		if d := Detect(dir); !strings.HasPrefix(d.Install, want+" install") {
			t.Errorf("%q: install = %q, want prefix %q", lock, d.Install, want)
		}
	}
}

func TestStaticSiteInSrc(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"src/index.html": "x", "README.md": "y"})
	if d := Detect(dir); d.OutputDirs[0] != "src" {
		t.Fatalf("got %v", d.OutputDirs)
	}
}

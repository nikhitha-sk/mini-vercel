package builder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Detection is what the worker learns by reading the source tree before the
// build, so that most real-world repositories deploy with zero config.
type Detection struct {
	Framework      string   // preset id, e.g. "vite", "nextjs", "static"
	FrameworkName  string   // human name for logs
	HasPackageJSON bool     //
	HasBuildScript bool     // package.json has a "build" script
	PackageManager string   // npm | pnpm | yarn | bun
	Install        string   // install command for the detected package manager
	Build          string   // build command ("" = no build step)
	OutputDirs     []string // candidate output directories, most likely first
	NodeMajor      int      // from engines.node / .nvmrc / .node-version (0 = default)
	Warnings       []string
}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	PackageManager  string            `json:"packageManager"`
	Engines         struct {
		Node string `json:"node"`
	} `json:"engines"`
}

type fwRule struct {
	id, name string
	deps     []string
	outputs  []string
	build    string // override build command when the repo has no build script
}

// Ordered: meta-frameworks before the bundlers they use.
var fwRules = []fwRule{
	{id: "nextjs", name: "Next.js", deps: []string{"next"}, outputs: []string{"out"}},
	{id: "nuxt", name: "Nuxt", deps: []string{"nuxt"}, outputs: []string{".output/public", "dist"}, build: "npx nuxi generate"},
	{id: "astro", name: "Astro", deps: []string{"astro"}, outputs: []string{"dist"}},
	{id: "sveltekit", name: "SvelteKit", deps: []string{"@sveltejs/kit"}, outputs: []string{"build"}},
	{id: "gatsby", name: "Gatsby", deps: []string{"gatsby"}, outputs: []string{"public"}},
	{id: "docusaurus", name: "Docusaurus", deps: []string{"@docusaurus/core"}, outputs: []string{"build"}},
	{id: "vitepress", name: "VitePress", deps: []string{"vitepress"}, outputs: []string{"docs/.vitepress/dist", ".vitepress/dist"}},
	{id: "vuepress", name: "VuePress", deps: []string{"vuepress", "@vuepress/cli"}, outputs: []string{"docs/.vuepress/dist", ".vuepress/dist"}},
	{id: "angular", name: "Angular", deps: []string{"@angular/core"}, outputs: []string{"dist/*/browser", "dist/*"}},
	{id: "cra", name: "Create React App", deps: []string{"react-scripts"}, outputs: []string{"build"}},
	{id: "vue-cli", name: "Vue CLI", deps: []string{"@vue/cli-service"}, outputs: []string{"dist"}},
	{id: "eleventy", name: "Eleventy", deps: []string{"@11ty/eleventy"}, outputs: []string{"_site"}},
	{id: "hexo", name: "Hexo", deps: []string{"hexo"}, outputs: []string{"public"}},
	{id: "vite", name: "Vite", deps: []string{"vite"}, outputs: []string{"dist"}},
	{id: "parcel", name: "Parcel", deps: []string{"parcel", "parcel-bundler"}, outputs: []string{"dist"}},
	{id: "webpack", name: "webpack", deps: []string{"webpack"}, outputs: []string{"dist", "build"}},
}

// Generic fallbacks searched after the build when the expected folder is missing.
var commonOutputs = []string{"dist", "build", "out", "public", "_site", ".output/public", "www", "docs"}

// Detect inspects dir (the project root inside the source tree).
func Detect(dir string) Detection {
	d := Detection{Framework: "static", FrameworkName: "Static HTML"}
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		// No package.json: a plain static site. Serve the root, or public/ if
		// the HTML lives there.
		d.OutputDirs = []string{"."}
		if !exists(filepath.Join(dir, "index.html")) {
			for _, sub := range []string{"public", "src", "docs", "site", "www", "dist", "build"} {
				if exists(filepath.Join(dir, sub, "index.html")) {
					d.OutputDirs = []string{sub, "."}
					break
				}
			}
		}
		return d
	}
	d.HasPackageJSON = true
	var pkg packageJSON
	if err := json.Unmarshal(raw, &pkg); err != nil {
		d.Warnings = append(d.Warnings, "package.json could not be parsed: "+err.Error())
	}
	deps := map[string]bool{}
	for k := range pkg.Dependencies {
		deps[k] = true
	}
	for k := range pkg.DevDependencies {
		deps[k] = true
	}

	d.PackageManager, d.Install = installFor(dir, pkg.PackageManager)
	_, d.HasBuildScript = pkg.Scripts["build"]
	if d.HasBuildScript {
		d.Build = runScript(d.PackageManager, "build")
	}

	d.Framework, d.FrameworkName = "node", "Node.js project"
	for _, r := range fwRules {
		if anyDep(deps, r.deps) {
			d.Framework, d.FrameworkName = r.id, r.name
			d.OutputDirs = append(d.OutputDirs, r.outputs...)
			if r.id == "nuxt" && pkg.Scripts["generate"] != "" {
				d.Build = runScript(d.PackageManager, "generate")
			} else if !d.HasBuildScript && r.build != "" {
				d.Build = r.build
			}
			break
		}
	}
	if d.Framework == "nextjs" && !nextStaticExport(dir) {
		d.Warnings = append(d.Warnings, "Next.js app without output: 'export' in next.config. Only static exports can be hosted; pages that need a server (API routes, SSR) will not work")
	}
	if !d.HasBuildScript && d.Build == "" {
		d.OutputDirs = append(d.OutputDirs, "public", ".", "src")
	}
	d.OutputDirs = append(d.OutputDirs, commonOutputs...)
	d.OutputDirs = dedupe(d.OutputDirs)
	d.NodeMajor = nodeVersion(dir, pkg.Engines.Node)
	return d
}

// installFor picks the package manager from the lockfile / packageManager
// field. pnpm and modern yarn run through corepack, which reads the exact
// version from package.json, without needing root to enable it.
func installFor(dir, packageManager string) (pm, cmd string) {
	switch {
	case exists(filepath.Join(dir, "pnpm-lock.yaml")) || strings.HasPrefix(packageManager, "pnpm"):
		bin := pnpmBinary(dir, packageManager)
		return "pnpm", bin + " install --frozen-lockfile || " + bin + " install --no-frozen-lockfile"
	case exists(filepath.Join(dir, "bun.lockb")) || exists(filepath.Join(dir, "bun.lock")) || strings.HasPrefix(packageManager, "bun"):
		return "bun", "npx --yes bun install"
	case exists(filepath.Join(dir, "yarn.lock")) || strings.HasPrefix(packageManager, "yarn"):
		return "yarn", "yarn install"
	case exists(filepath.Join(dir, "package-lock.json")) || exists(filepath.Join(dir, "npm-shrinkwrap.json")):
		return "npm", "npm ci || npm install"
	default:
		return "npm", "npm install"
	}
}

var (
	lockVersionRe = regexp.MustCompile(`(?m)^lockfileVersion:\s*'?(\d+)(?:\.(\d+))?`)
	pmMajorRe     = regexp.MustCompile(`^pnpm@(\d+)`)
)

// pnpmBinary returns the pnpm invocation that matches the repository. With a
// "packageManager" field corepack already picks the exact version; otherwise
// the lockfile format tells us which major wrote it (5.3→6, 5.4→7, 6→8, 9→9).
// pnpm 10+ blocks dependency build scripts by default; the sandbox is the
// security boundary here, so they are allowed (as on Vercel).
func pnpmBinary(dir, packageManager string) string {
	if m := pmMajorRe.FindStringSubmatch(packageManager); m != nil {
		if n, _ := strconv.Atoi(m[1]); n >= 10 {
			return "pnpm --config.dangerously-allow-all-builds=true"
		}
		return "pnpm"
	}
	b, err := os.ReadFile(filepath.Join(dir, "pnpm-lock.yaml"))
	if err != nil {
		return "corepack pnpm@9"
	}
	m := lockVersionRe.FindSubmatch(b)
	if m == nil {
		return "corepack pnpm@9"
	}
	major, _ := strconv.Atoi(string(m[1]))
	minor, _ := strconv.Atoi(string(m[2]))
	switch {
	case major == 5 && minor <= 3:
		return "corepack pnpm@6"
	case major == 5:
		return "corepack pnpm@7"
	case major == 6:
		return "corepack pnpm@8"
	default:
		return "corepack pnpm@9"
	}
}

func runScript(pm, script string) string {
	switch pm {
	case "pnpm":
		return "npm run " + script // node_modules/.bin is on PATH; works with any pnpm layout
	case "yarn":
		return "yarn run " + script
	case "bun":
		return "npx --yes bun run " + script
	default:
		return "npm run " + script
	}
}

var nextExportRe = regexp.MustCompile(`output\s*:\s*['"]export['"]`)

func nextStaticExport(dir string) bool {
	for _, f := range []string{"next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs"} {
		if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil && nextExportRe.Match(b) {
			return true
		}
	}
	return false
}

var majorRe = regexp.MustCompile(`(\d+)`)

// nodeVersion returns a supported Node major (18, 20, 22, 24) or 0.
func nodeVersion(dir, engines string) int {
	candidates := []string{}
	for _, f := range []string{".nvmrc", ".node-version"} {
		if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
			candidates = append(candidates, strings.TrimSpace(string(b)))
		}
	}
	candidates = append(candidates, engines)
	for _, c := range candidates {
		c = strings.TrimPrefix(strings.ToLower(c), "v")
		if c == "" || strings.HasPrefix(c, "lts") || c == "node" {
			continue
		}
		m := majorRe.FindString(c)
		if m == "" {
			continue
		}
		n, _ := strconv.Atoi(m)
		switch {
		case n <= 18:
			return 18
		case n <= 20:
			return 20
		case n <= 22:
			return 22
		default:
			return 24
		}
	}
	return 0
}

func anyDep(deps map[string]bool, names []string) bool {
	for _, n := range names {
		if deps[n] {
			return true
		}
	}
	return false
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

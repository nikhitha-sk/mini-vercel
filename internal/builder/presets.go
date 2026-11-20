package builder

// AutoInstall picks the package manager from the lockfile in the repo.
const AutoInstall = `if [ -f pnpm-lock.yaml ]; then npx --yes pnpm@9 install --frozen-lockfile; ` +
	`elif [ -f yarn.lock ]; then yarn install --frozen-lockfile --non-interactive; ` +
	`elif [ -f bun.lockb ] || [ -f bun.lock ]; then npx --yes bun install; ` +
	`elif [ -f package-lock.json ]; then npm ci; ` +
	`elif [ -f package.json ]; then npm install; ` +
	`else echo "No package.json found, skipping install"; fi`

type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Install     string `json:"installCommand"`
	Build       string `json:"buildCommand"`
	OutputDir   string `json:"outputDirectory"`
}

// Presets are framework defaults. Static output only: no SSR, no functions.
var Presets = []Preset{
	{ID: "vite", Name: "Vite", Description: "React, Vue, Svelte or vanilla apps built with Vite", Build: "npm run build", OutputDir: "dist"},
	{ID: "nextjs", Name: "Next.js (static export)", Description: "Next.js with output: 'export'", Build: "npm run build", OutputDir: "out"},
	{ID: "astro", Name: "Astro", Description: "Content-focused static sites", Build: "npm run build", OutputDir: "dist"},
	{ID: "cra", Name: "Create React App", Description: "Legacy react-scripts projects", Build: "npm run build", OutputDir: "build"},
	{ID: "docusaurus", Name: "Docusaurus", Description: "Documentation sites", Build: "npm run build", OutputDir: "build"},
	{ID: "static", Name: "Other / Static HTML", Description: "Plain HTML, CSS and JS. No build step", Build: "", OutputDir: "."},
}

func PresetByID(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

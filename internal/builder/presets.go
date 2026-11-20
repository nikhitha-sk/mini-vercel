package builder

type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Install     string `json:"installCommand"`
	Build       string `json:"buildCommand"`
	OutputDir   string `json:"outputDirectory"`
}

// Presets are shown in the dashboard. "auto" (the default) lets the worker
// detect the framework, package manager, Node version and output folder from
// the source; the others document each framework's defaults. Explicit
// install/build/output values in project settings always win.
var Presets = []Preset{
	{ID: "auto", Name: "Auto-detect (recommended)", Description: "Reads package.json and lockfiles to pick the framework, package manager, Node version and output folder"},
	{ID: "vite", Name: "Vite", Description: "React, Vue, Svelte or vanilla apps built with Vite", Build: "npm run build", OutputDir: "dist"},
	{ID: "nextjs", Name: "Next.js (static export)", Description: "Requires output: 'export' in next.config", Build: "npm run build", OutputDir: "out"},
	{ID: "astro", Name: "Astro", Description: "Content-focused static sites", Build: "npm run build", OutputDir: "dist"},
	{ID: "cra", Name: "Create React App", Description: "Legacy react-scripts projects", Build: "npm run build", OutputDir: "build"},
	{ID: "angular", Name: "Angular", Description: "Angular CLI applications", Build: "npm run build", OutputDir: "dist/<name>/browser"},
	{ID: "sveltekit", Name: "SvelteKit (static)", Description: "Requires @sveltejs/adapter-static", Build: "npm run build", OutputDir: "build"},
	{ID: "docusaurus", Name: "Docusaurus", Description: "Documentation sites", Build: "npm run build", OutputDir: "build"},
	{ID: "gatsby", Name: "Gatsby", Description: "React static site generator", Build: "npm run build", OutputDir: "public"},
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

export function timeAgo(iso: string | null | undefined): string {
  if (!iso) return "–";
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 10) return "just now";
  if (s < 60) return `${Math.floor(s)}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  if (s < 86400 * 30) return `${Math.floor(s / 86400)}d ago`;
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

export function duration(ms: number | null | undefined): string {
  if (ms == null) return "–";
  if (ms < 1000) return `${ms}ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${Math.round(s % 60)}s`;
}

export function bytes(n: number | null | undefined): string {
  if (!n) return "–";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

export function shortSha(sha: string): string {
  return sha ? sha.slice(0, 7) : "";
}

export function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

export function repoPath(url: string): string {
  return url.replace(/^https?:\/\//, "").replace(/^github\.com\//, "").replace(/\.git$/, "");
}

export function clockTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString(undefined, { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function cn(...c: (string | false | null | undefined)[]): string {
  return c.filter(Boolean).join(" ");
}

const FRAMEWORKS: Record<string, string> = {
  static: "Static HTML", node: "Node.js", nextjs: "Next.js", nuxt: "Nuxt", astro: "Astro", sveltekit: "SvelteKit", gatsby: "Gatsby",
  docusaurus: "Docusaurus", vitepress: "VitePress", vuepress: "VuePress", angular: "Angular", cra: "Create React App",
  "vue-cli": "Vue CLI", eleventy: "Eleventy", hexo: "Hexo", vite: "Vite", parcel: "Parcel", webpack: "webpack",
};
export function frameworkName(id: string) {
  return FRAMEWORKS[id] ?? id;
}

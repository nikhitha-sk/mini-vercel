"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { ChevronDown, FileArchive, Github, Lock, Minus, Plus, Rocket, ShieldCheck, Upload } from "lucide-react";
import { AppShell, Container } from "@/components/shell";
import { Button, Card, Input, Label, Select } from "@/components/ui";
import { useFeedback } from "@/components/feedback";
import { api, type PlatformConfig, type Preset } from "@/lib/api";
import { bytes, cn } from "@/lib/format";

type Source = "git" | "zip";
interface EnvRow {
  key: string;
  value: string;
}

const EXAMPLES = [
  { label: "Vite demo (this repo)", repo: "https://github.com/nikhitha-sk/mini-vercel", root: "examples/vite-demo", framework: "vite" },
  { label: "Static HTML demo (this repo)", repo: "https://github.com/nikhitha-sk/mini-vercel", root: "examples/static-site", framework: "static" },
];

export default function NewProjectPage() {
  const router = useRouter();
  const { toast } = useFeedback();
  const [presets, setPresets] = useState<Preset[]>([]);
  const [cfg, setCfg] = useState<PlatformConfig | null>(null);
  const [source, setSource] = useState<Source>("git");
  const [repoUrl, setRepoUrl] = useState("");
  const [branch, setBranch] = useState("main");
  const [name, setName] = useState("");
  const [framework, setFramework] = useState("vite");
  const [rootDir, setRootDir] = useState("");
  const [installCmd, setInstallCmd] = useState("");
  const [buildCmd, setBuildCmd] = useState("");
  const [outputDir, setOutputDir] = useState("");
  const [env, setEnv] = useState<EnvRow[]>([{ key: "", value: "" }]);
  const [file, setFile] = useState<File | null>(null);
  const [showBuild, setShowBuild] = useState(false);
  const [showEnv, setShowEnv] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.presets().then(setPresets).catch(() => {});
    api.config().then(setCfg).catch(() => {});
  }, []);

  const preset = presets.find((p) => p.id === framework);

  // Derive a project name from the repo URL until the user types one.
  const [nameTouched, setNameTouched] = useState(false);
  useEffect(() => {
    if (nameTouched) return;
    if (source === "git") {
      const m = repoUrl.replace(/\.git$/, "").match(/([^/]+)\/?$/);
      setName(rootDir ? rootDir.split("/").filter(Boolean).pop() ?? "" : m ? m[1] : "");
    } else if (file) {
      setName(file.name.replace(/\.zip$/i, ""));
    }
  }, [repoUrl, rootDir, file, source, nameTouched]);

  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
  const previewHost = slug && cfg ? `${slug}.${cfg.rootDomain}${cfg.publicPort && !["80", "443"].includes(cfg.publicPort) ? `:${cfg.publicPort}` : ""}` : "";

  const pickFile = (f: File | undefined | null) => {
    if (!f) return;
    if (!f.name.toLowerCase().endsWith(".zip")) {
      toast("error", "Unsupported file", "Please choose a .zip archive.");
      return;
    }
    if (f.size > 100 * 1024 * 1024) {
      toast("error", "File too large", "Zip archives must be 100 MB or smaller.");
      return;
    }
    setFile(f);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (source === "git" && !repoUrl.trim()) return setError("Enter a Git repository URL.");
    if (source === "zip" && !file) return setError("Choose a .zip archive to upload.");
    if (!name.trim()) return setError("Enter a project name.");
    setSubmitting(true);
    try {
      const body: Record<string, unknown> = {
        name: name.trim(),
        framework,
        branch: branch.trim() || "main",
        rootDirectory: rootDir.trim(),
        env: env.filter((r) => r.key.trim()).map((r) => ({ key: r.key.trim(), value: r.value })),
        deploy: source === "git",
      };
      if (source === "git") body.repoUrl = repoUrl.trim();
      if (installCmd.trim()) body.installCommand = installCmd.trim();
      if (buildCmd.trim()) body.buildCommand = buildCmd.trim();
      if (outputDir.trim()) body.outputDirectory = outputDir.trim();

      const res = await api.createProject(body);
      let depId = res.deployment?.id;
      if (source === "zip" && file) {
        const d = await api.deployZip(res.project.id, file);
        depId = d.id;
      }
      toast("success", "Deployment started", `${res.project.name} is building.`);
      router.push(depId ? `/deployment?id=${depId}` : `/project?id=${res.project.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setSubmitting(false);
    }
  };

  return (
    <AppShell crumbs={[{ label: "New Project" }]} tabs={[]}>
      <div className="border-b border-line bg-elevated">
        <div className="mx-auto max-w-3xl px-4 py-10 sm:px-6">
          <h1 className="text-[32px] font-semibold tracking-tight">Let&apos;s build something new.</h1>
          <p className="mt-2 text-sm text-muted">Import a Git repository or upload a zip. Static output only (Vite, Astro, Next.js export, plain HTML).</p>
        </div>
      </div>
      <Container className="max-w-3xl">
        <form onSubmit={submit} className="space-y-6">
          <Card>
            <div className="border-b border-line p-5">
              <h2 className="text-[15px] font-semibold">Source</h2>
              <div className="mt-4 grid grid-cols-2 gap-1 rounded-lg border border-line bg-subtle p-1">
                {(
                  [
                    { v: "git", label: "Import Git Repository", icon: <Github className="size-4" /> },
                    { v: "zip", label: "Upload Zip", icon: <FileArchive className="size-4" /> },
                  ] as const
                ).map((o) => (
                  <button
                    type="button"
                    key={o.v}
                    onClick={() => setSource(o.v)}
                    className={cn(
                      "flex h-9 cursor-pointer items-center justify-center gap-2 rounded-md text-[13px] font-medium transition-all",
                      source === o.v ? "bg-elevated text-fg shadow-card ring-1 ring-line" : "text-muted hover:text-fg",
                    )}
                  >
                    {o.icon}
                    {o.label}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-5 p-5">
              {source === "git" ? (
                <>
                  <div>
                    <Label htmlFor="repo" hint="Public HTTPS repositories">
                      Repository URL
                    </Label>
                    <div className="relative">
                      <Github className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-faint" />
                      <Input
                        id="repo"
                        value={repoUrl}
                        onChange={(e) => setRepoUrl(e.target.value)}
                        placeholder="https://github.com/username/repository"
                        className="pl-9"
                        autoFocus
                      />
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted">
                      <span>Try an example:</span>
                      {EXAMPLES.map((ex) => (
                        <button
                          type="button"
                          key={ex.label}
                          onClick={() => {
                            setRepoUrl(ex.repo);
                            setRootDir(ex.root);
                            setFramework(ex.framework);
                            setBranch("main");
                            setNameTouched(false);
                          }}
                          className="cursor-pointer rounded-full border border-line px-2.5 py-0.5 hover:border-line-strong hover:text-fg"
                        >
                          {ex.label}
                        </button>
                      ))}
                    </div>
                  </div>
                  <div className="grid gap-5 sm:grid-cols-2">
                    <div>
                      <Label htmlFor="branch">Branch</Label>
                      <Input id="branch" value={branch} onChange={(e) => setBranch(e.target.value)} placeholder="main" mono />
                    </div>
                    <div>
                      <Label htmlFor="root" hint="Optional">
                        Root Directory
                      </Label>
                      <Input id="root" value={rootDir} onChange={(e) => setRootDir(e.target.value)} placeholder="./" mono />
                    </div>
                  </div>
                </>
              ) : (
                <div>
                  <Label>Archive</Label>
                  <div
                    onDragOver={(e) => {
                      e.preventDefault();
                      setDragOver(true);
                    }}
                    onDragLeave={() => setDragOver(false)}
                    onDrop={(e) => {
                      e.preventDefault();
                      setDragOver(false);
                      pickFile(e.dataTransfer.files[0]);
                    }}
                    onClick={() => fileInput.current?.click()}
                    className={cn(
                      "flex cursor-pointer flex-col items-center justify-center rounded-lg border border-dashed px-6 py-10 text-center transition-colors",
                      dragOver ? "border-ring bg-info-bg" : "border-line-strong hover:bg-subtle",
                    )}
                  >
                    <div className="flex size-10 items-center justify-center rounded-full border border-line bg-elevated text-muted">
                      <Upload className="size-4" />
                    </div>
                    {file ? (
                      <>
                        <p className="mt-3 text-sm font-medium">{file.name}</p>
                        <p className="mt-0.5 text-xs text-muted">{bytes(file.size)} · click to replace</p>
                      </>
                    ) : (
                      <>
                        <p className="mt-3 text-sm font-medium">Drop a .zip here, or click to browse</p>
                        <p className="mt-0.5 text-xs text-muted">Up to 100 MB. A single top-level folder is used as the project root.</p>
                      </>
                    )}
                    <input ref={fileInput} type="file" accept=".zip,application/zip" hidden onChange={(e) => pickFile(e.target.files?.[0])} />
                  </div>
                  <div className="mt-5">
                    <Label htmlFor="root2" hint="Optional">
                      Root Directory
                    </Label>
                    <Input id="root2" value={rootDir} onChange={(e) => setRootDir(e.target.value)} placeholder="./" mono />
                  </div>
                </div>
              )}
            </div>
          </Card>

          <Card>
            <div className="space-y-5 p-5">
              <h2 className="text-[15px] font-semibold">Configure Project</h2>
              <div className="grid gap-5 sm:grid-cols-2">
                <div>
                  <Label htmlFor="name">Project Name</Label>
                  <Input
                    id="name"
                    value={name}
                    onChange={(e) => {
                      setNameTouched(true);
                      setName(e.target.value);
                    }}
                    placeholder="my-portfolio"
                  />
                  {previewHost && <p className="mt-1.5 truncate font-mono text-xs text-muted">{previewHost}</p>}
                </div>
                <div>
                  <Label htmlFor="fw">Framework Preset</Label>
                  <Select id="fw" value={framework} onChange={(e) => setFramework(e.target.value)}>
                    {(presets.length ? presets : [{ id: "vite", name: "Vite" } as Preset]).map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </Select>
                  {preset && <p className="mt-1.5 text-xs text-muted">{preset.description}</p>}
                </div>
              </div>

              <Collapsible title="Build and Output Settings" open={showBuild} onToggle={() => setShowBuild((v) => !v)}>
                <div className="space-y-4">
                  <OverrideField
                    label="Install Command"
                    value={installCmd}
                    onChange={setInstallCmd}
                    placeholder="Auto-detect: npm ci · yarn · pnpm · bun"
                  />
                  <OverrideField label="Build Command" value={buildCmd} onChange={setBuildCmd} placeholder={preset?.buildCommand || "None"} />
                  <OverrideField label="Output Directory" value={outputDir} onChange={setOutputDir} placeholder={preset?.outputDirectory || "dist"} />
                </div>
              </Collapsible>

              <Collapsible title="Environment Variables" open={showEnv} onToggle={() => setShowEnv((v) => !v)}>
                <p className="mb-3 flex items-center gap-1.5 text-xs text-muted">
                  <Lock className="size-3.5" /> Encrypted at rest with AES-256-GCM. Only decrypted inside the build container.
                </p>
                <div className="space-y-2">
                  {env.map((row, i) => (
                    <div key={i} className="flex gap-2">
                      <Input
                        value={row.key}
                        onChange={(e) => setEnv(env.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)))}
                        placeholder="EXAMPLE_NAME"
                        mono
                        aria-label="Key"
                      />
                      <Input
                        value={row.value}
                        onChange={(e) => setEnv(env.map((r, j) => (j === i ? { ...r, value: e.target.value } : r)))}
                        placeholder="value"
                        mono
                        aria-label="Value"
                      />
                      <Button
                        type="button"
                        variant="secondary"
                        className="w-10 shrink-0 px-0"
                        aria-label="Remove variable"
                        onClick={() => setEnv(env.length === 1 ? [{ key: "", value: "" }] : env.filter((_, j) => j !== i))}
                      >
                        <Minus className="size-4" />
                      </Button>
                    </div>
                  ))}
                  <Button type="button" variant="ghost" size="sm" icon={<Plus className="size-4" />} onClick={() => setEnv([...env, { key: "", value: "" }])}>
                    Add another
                  </Button>
                </div>
              </Collapsible>
            </div>
          </Card>

          {error && <div className="rounded-lg border border-err/30 bg-err-bg px-4 py-3 text-[13px] text-err">{error}</div>}

          <div className="flex flex-col-reverse items-stretch justify-between gap-4 sm:flex-row sm:items-center">
            {cfg && (
              <p className="flex items-center gap-1.5 text-xs text-muted">
                <ShieldCheck className="size-4 text-ok" />
                Sandboxed build: {cfg.limits.cpus} vCPU · {cfg.limits.memoryMB} MB · {Math.round(cfg.limits.timeoutSec / 60)} min · non-root
              </p>
            )}
            <Button type="submit" variant="primary" loading={submitting} icon={<Rocket className="size-4" />} className="sm:min-w-40">
              Deploy
            </Button>
          </div>
        </form>
      </Container>
    </AppShell>
  );
}

function Collapsible({ title, open, onToggle, children }: { title: string; open: boolean; onToggle: () => void; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-line">
      <button type="button" onClick={onToggle} className="flex w-full cursor-pointer items-center justify-between px-4 py-3 text-left text-[13px] font-medium">
        {title}
        <ChevronDown className={cn("size-4 text-muted transition-transform", open && "rotate-180")} />
      </button>
      {open && <div className="border-t border-line p-4">{children}</div>}
    </div>
  );
}

function OverrideField({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (v: string) => void; placeholder: string }) {
  return (
    <div>
      <Label hint={value ? "Override" : "Default"}>{label}</Label>
      <Input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} mono />
    </div>
  );
}

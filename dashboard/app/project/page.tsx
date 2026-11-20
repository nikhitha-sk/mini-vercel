"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useRef, useState } from "react";
import { ExternalLink, FileArchive, GitBranch, GitCommitHorizontal, Github, Globe, KeyRound, Lock, Plus, Rocket, Trash2, Upload, Webhook } from "lucide-react";
import { AppShell, Container, PageHeader } from "@/components/shell";
import { Button, ButtonLink, Card, CardHeader, EmptyState, ErrorBanner, Input, Kv, Label, Select, Skeleton, StatusBadge } from "@/components/ui";
import { CopyButton, useFeedback } from "@/components/feedback";
import { DeploymentList } from "@/components/deployment-list";
import { API_URL, api, isActive, type Preset, type Project } from "@/lib/api";
import { useResource, useTick } from "@/lib/hooks";
import { duration, hostOf, repoPath, shortSha, timeAgo } from "@/lib/format";

export default function ProjectPage() {
  return (
    <Suspense>
      <ProjectView />
    </Suspense>
  );
}

type TabId = "overview" | "deployments" | "env" | "settings";

function ProjectView() {
  useTick();
  const params = useSearchParams();
  const router = useRouter();
  const id = params.get("id") ?? "";
  const tab = (params.get("tab") as TabId) || "overview";
  const { toast } = useFeedback();
  const zipInput = useRef<HTMLInputElement>(null);
  const [deploying, setDeploying] = useState(false);

  const project = useResource(() => api.project(id), [id], {
    pollMs: 2500,
    pollWhile: (p) => !!p.latestDeployment && isActive(p.latestDeployment.status),
  });
  const deployments = useResource(() => api.deployments(id), [id], {
    pollMs: 2500,
    pollWhile: (ds) => ds.some((d) => isActive(d.status)),
  });
  const reloadAll = () => {
    project.reload();
    deployments.reload();
  };

  const p = project.data;
  const href = (t: TabId) => `/project?id=${id}${t === "overview" ? "" : `&tab=${t}`}`;
  const tabs = [
    { label: "Project", href: href("overview"), active: tab === "overview" },
    { label: "Deployments", href: href("deployments"), active: tab === "deployments" },
    { label: "Environment Variables", href: href("env"), active: tab === "env" },
    { label: "Settings", href: href("settings"), active: tab === "settings" },
  ];

  const deployGit = async () => {
    setDeploying(true);
    try {
      const d = await api.deploy(id);
      toast("success", "Deployment queued");
      router.push(`/deployment?id=${d.id}`);
    } catch (e) {
      toast("error", "Could not deploy", e instanceof Error ? e.message : String(e));
      setDeploying(false);
    }
  };
  const deployZip = async (f: File | undefined) => {
    if (!f) return;
    setDeploying(true);
    try {
      const d = await api.deployZip(id, f);
      toast("success", "Upload complete", "Your archive is building.");
      router.push(`/deployment?id=${d.id}`);
    } catch (e) {
      toast("error", "Upload failed", e instanceof Error ? e.message : String(e));
      setDeploying(false);
    }
  };

  return (
    <AppShell crumbs={[{ label: p?.name ?? "Project", href: href("overview") }]} tabs={tabs}>
      {project.error && (
        <Container>
          <ErrorBanner message={project.error} onRetry={project.reload} />
        </Container>
      )}
      {!p && !project.error && (
        <Container>
          <Skeleton className="h-10 w-64" />
          <Skeleton className="mt-8 h-72 w-full" />
        </Container>
      )}
      {p && (
        <>
          <PageHeader
            title={p.name}
            description={
              p.repoUrl ? (
                <a href={p.repoUrl} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 hover:text-fg">
                  <Github className="size-4" /> {repoPath(p.repoUrl)}
                </a>
              ) : (
                <span className="inline-flex items-center gap-1.5">
                  <FileArchive className="size-4" /> Deployed from zip uploads
                </span>
              )
            }
            actions={
              <>
                <input ref={zipInput} type="file" accept=".zip" hidden onChange={(e) => deployZip(e.target.files?.[0])} />
                <Button variant="secondary" icon={<Upload className="size-4" />} onClick={() => zipInput.current?.click()} disabled={deploying}>
                  Upload Zip
                </Button>
                {p.repoUrl && (
                  <Button variant="secondary" icon={<Rocket className="size-4" />} onClick={deployGit} loading={deploying}>
                    Deploy
                  </Button>
                )}
                {p.productionDeployment && (
                  <ButtonLink href={p.productionUrl} external variant="primary" icon={<ExternalLink className="size-4" />}>
                    Visit
                  </ButtonLink>
                )}
              </>
            }
          />
          <Container>
            {tab === "overview" && <OverviewTab p={p} />}
            {tab === "deployments" && (
              <Card className="overflow-hidden">
                <CardHeader
                  title="Deployments"
                  description="Every deployment is immutable and keeps its own URL. Promote any ready deployment to roll back instantly."
                />
                {deployments.data?.length === 0 && <EmptyState icon={<Rocket className="size-5" />} title="No deployments yet" />}
                {deployments.data && <DeploymentList deployments={deployments.data} onChanged={reloadAll} />}
                {!deployments.data && <div className="space-y-3 p-5">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-12" />)}</div>}
              </Card>
            )}
            {tab === "env" && <EnvTab projectId={p.id} />}
            {tab === "settings" && <SettingsTab p={p} onSaved={project.reload} />}
          </Container>
        </>
      )}
    </AppShell>
  );
}

function OverviewTab({ p }: { p: Project }) {
  const d = p.productionDeployment;
  const latest = p.latestDeployment;
  return (
    <div className="space-y-6">
      {latest && isActive(latest.status) && (
        <Link
          href={`/deployment?id=${latest.id}`}
          className="flex items-center justify-between gap-3 rounded-xl border border-warn/30 bg-warn-bg px-4 py-3 text-[13px] text-warn"
        >
          <span className="flex items-center gap-2">
            <StatusBadge status={latest.status} /> A new deployment is in progress.
          </span>
          <span className="font-medium underline underline-offset-2">View build logs</span>
        </Link>
      )}

      <Card className="overflow-hidden">
        <CardHeader
          title="Production Deployment"
          description="The deployment that is available to your visitors."
          actions={
            d && (
              <ButtonLink href={`/deployment?id=${d.id}`} size="sm">
                Build Logs
              </ButtonLink>
            )
          }
        />
        {d ? (
          <div className="grid grid-cols-1 gap-6 p-5 md:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
            <a
              href={p.productionUrl}
              target="_blank"
              rel="noreferrer"
              className="group relative block aspect-[16/10] overflow-hidden rounded-lg border border-line bg-subtle"
              aria-label="Open production site"
            >
              <iframe
                src={p.productionUrl}
                title={`${p.name} preview`}
                className="pointer-events-none absolute left-0 top-0 h-[250%] w-[250%] origin-top-left scale-[0.4] border-0 bg-white"
                loading="lazy"
                sandbox="allow-scripts allow-same-origin"
                tabIndex={-1}
              />
              <div className="absolute inset-0 bg-black/0 transition-colors group-hover:bg-black/5" />
            </a>
            <div className="grid content-start gap-5 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <Kv label="Deployment">
                  <a href={d.url} target="_blank" rel="noreferrer" className="font-mono text-[13px] hover:underline">
                    {hostOf(d.url)}
                  </a>
                </Kv>
              </div>
              <div className="sm:col-span-2">
                <div className="mb-1 text-xs font-medium text-faint">Domains</div>
                <a href={p.productionUrl} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 text-sm font-medium hover:underline">
                  <Globe className="size-3.5 text-muted" />
                  {hostOf(p.productionUrl)}
                  <ExternalLink className="size-3 text-muted" />
                </a>
              </div>
              <Kv label="Status">
                <StatusBadge status={d.status} />
              </Kv>
              <Kv label="Created">
                {timeAgo(d.createdAt)} <span className="text-muted">· {duration(d.durationMs)}</span>
              </Kv>
              <div className="sm:col-span-2">
                <div className="mb-1 text-xs font-medium text-faint">Source</div>
                {d.source === "zip" ? (
                  <div className="flex items-center gap-1.5 text-sm">
                    <FileArchive className="size-3.5 text-muted" /> Zip upload
                  </div>
                ) : (
                  <div className="space-y-1 text-sm">
                    <div className="flex items-center gap-1.5">
                      <GitBranch className="size-3.5 text-muted" />
                      <span className="font-mono text-[13px]">{d.branch}</span>
                    </div>
                    <div className="flex min-w-0 items-center gap-1.5">
                      <GitCommitHorizontal className="size-3.5 shrink-0 text-muted" />
                      <span className="font-mono text-[13px] text-muted">{shortSha(d.commitSha)}</span>
                      <span className="truncate">{d.commitMessage}</span>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        ) : (
          <EmptyState
            icon={<Rocket className="size-5" />}
            title="No production deployment yet"
            description={latest ? "Your first deployment is on its way." : "Trigger a deployment to go live."}
            action={latest && <ButtonLink href={`/deployment?id=${latest.id}`}>View latest deployment</ButtonLink>}
          />
        )}
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <InfoTile title="Build settings" lines={[`Framework: ${p.framework}`, `Output: ${p.outputDirectory}`, p.rootDirectory ? `Root: ${p.rootDirectory}` : "Root: ./"]} href={`/project?id=${p.id}&tab=settings`} />
        <InfoTile title="Git" lines={p.repoUrl ? [repoPath(p.repoUrl), `Production branch: ${p.branch}`] : ["Not connected"]} href={`/project?id=${p.id}&tab=settings`} />
        <InfoTile title="Deployments" lines={[latest ? `Latest: ${timeAgo(latest.createdAt)}` : "None yet", "Instant rollback available"]} href={`/project?id=${p.id}&tab=deployments`} />
      </div>
    </div>
  );
}

function InfoTile({ title, lines, href }: { title: string; lines: string[]; href: string }) {
  return (
    <Link href={href} className="rounded-xl border border-line bg-elevated p-4 shadow-card transition-colors hover:border-line-strong">
      <div className="text-[13px] font-semibold">{title}</div>
      {lines.map((l) => (
        <div key={l} className="mt-1 truncate text-[13px] text-muted">
          {l}
        </div>
      ))}
    </Link>
  );
}

function EnvTab({ projectId }: { projectId: string }) {
  const vars = useResource(() => api.env(projectId), [projectId]);
  const { toast, confirm } = useFeedback();
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!key.trim()) return;
    setSaving(true);
    try {
      await api.setEnv(projectId, key.trim(), value);
      toast("success", "Environment variable saved", "It will be available on the next deployment.");
      setKey("");
      setValue("");
      vars.reload();
    } catch (err) {
      toast("error", "Could not save", err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const remove = async (k: string) => {
    if (!(await confirm({ title: `Remove ${k}?`, description: "Future deployments will no longer receive this variable.", confirmLabel: "Remove", danger: true }))) return;
    try {
      await api.deleteEnv(projectId, k);
      vars.reload();
    } catch (err) {
      toast("error", "Could not remove", err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader
          title="Environment Variables"
          description={
            <span className="inline-flex items-center gap-1.5">
              <Lock className="size-3.5" /> Encrypted with AES-256-GCM and injected only into the build container. Values are write-only.
            </span>
          }
        />
        <form onSubmit={add} className="grid grid-cols-1 gap-3 p-5 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
          <div>
            <Label htmlFor="ek">Key</Label>
            <Input id="ek" value={key} onChange={(e) => setKey(e.target.value)} placeholder="VITE_API_URL" mono />
          </div>
          <div>
            <Label htmlFor="ev">Value</Label>
            <Input id="ev" value={value} onChange={(e) => setValue(e.target.value)} placeholder="https://api.example.com" mono />
          </div>
          <Button type="submit" variant="primary" loading={saving} icon={<Plus className="size-4" />}>
            Save
          </Button>
        </form>
      </Card>
      <Card className="overflow-hidden">
        {vars.data?.length === 0 && <EmptyState icon={<KeyRound className="size-5" />} title="No environment variables" description="Add API keys or config your build needs." />}
        <ul className="divide-y divide-line">
          {vars.data?.map((v) => (
            <li key={v.key} className="flex items-center gap-4 px-5 py-3.5">
              <KeyRound className="size-4 shrink-0 text-muted" />
              <span className="min-w-0 flex-1 truncate font-mono text-[13px] font-medium">{v.key}</span>
              <span className="hidden font-mono text-[13px] text-faint sm:inline">{v.preview}</span>
              <span className="hidden text-xs text-faint md:inline">Updated {timeAgo(v.updatedAt)}</span>
              <Button variant="ghost" size="sm" aria-label={`Remove ${v.key}`} onClick={() => remove(v.key)}>
                <Trash2 className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

function SettingsTab({ p, onSaved }: { p: Project; onSaved: () => void }) {
  const router = useRouter();
  const { toast, confirm } = useFeedback();
  const [presets, setPresets] = useState<Preset[]>([]);
  const [form, setForm] = useState({
    name: p.name,
    repoUrl: p.repoUrl,
    branch: p.branch,
    framework: p.framework,
    installCommand: p.installCommand,
    buildCommand: p.buildCommand,
    outputDirectory: p.outputDirectory,
    rootDirectory: p.rootDirectory,
  });
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    api.presets().then(setPresets).catch(() => {});
  }, []);
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setForm({ ...form, [k]: e.target.value });

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      await api.updateProject(p.id, form);
      toast("success", "Settings saved", "Changes apply to the next deployment.");
      onSaved();
    } catch (err) {
      toast("error", "Could not save", err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const del = async () => {
    const ok = await confirm({
      title: `Delete ${p.name}?`,
      description: "This removes the project, all deployments, environment variables and the production domain. This cannot be undone.",
      confirmLabel: "Delete Project",
      danger: true,
    });
    if (!ok) return;
    try {
      await api.deleteProject(p.id);
      toast("success", "Project deleted");
      router.push("/");
    } catch (err) {
      toast("error", "Could not delete", err instanceof Error ? err.message : String(err));
    }
  };

  const webhook = `${API_URL || (typeof window !== "undefined" ? window.location.origin : "")}${p.webhookUrl ?? ""}`;

  return (
    <div className="space-y-6">
      <form onSubmit={save}>
        <Card>
          <CardHeader title="General & Build" description="Settings are snapshotted into each deployment, so existing deployments never change." />
          <div className="grid grid-cols-1 gap-5 p-5 sm:grid-cols-2">
            <div>
              <Label htmlFor="s-name">Project Name</Label>
              <Input id="s-name" value={form.name} onChange={set("name")} />
            </div>
            <div>
              <Label htmlFor="s-fw">Framework Preset</Label>
              <Select id="s-fw" value={form.framework} onChange={set("framework")}>
                {presets.map((x) => (
                  <option key={x.id} value={x.id}>
                    {x.name}
                  </option>
                ))}
              </Select>
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="s-repo">Git Repository</Label>
              <Input id="s-repo" value={form.repoUrl} onChange={set("repoUrl")} placeholder="https://github.com/user/repo" />
            </div>
            <div>
              <Label htmlFor="s-branch">Production Branch</Label>
              <Input id="s-branch" value={form.branch} onChange={set("branch")} mono />
            </div>
            <div>
              <Label htmlFor="s-root">Root Directory</Label>
              <Input id="s-root" value={form.rootDirectory} onChange={set("rootDirectory")} placeholder="./" mono />
            </div>
            <div>
              <Label htmlFor="s-install" hint="Empty = auto-detect">
                Install Command
              </Label>
              <Input id="s-install" value={form.installCommand} onChange={set("installCommand")} placeholder="npm ci" mono />
            </div>
            <div>
              <Label htmlFor="s-build">Build Command</Label>
              <Input id="s-build" value={form.buildCommand} onChange={set("buildCommand")} placeholder="npm run build" mono />
            </div>
            <div>
              <Label htmlFor="s-out">Output Directory</Label>
              <Input id="s-out" value={form.outputDirectory} onChange={set("outputDirectory")} placeholder="dist" mono />
            </div>
          </div>
          <div className="flex items-center justify-between gap-4 border-t border-line bg-subtle px-5 py-3">
            <p className="text-xs text-muted">Project URL: {hostOf(p.productionUrl)}</p>
            <Button type="submit" variant="primary" size="sm" loading={saving}>
              Save
            </Button>
          </div>
        </Card>
      </form>

      {p.repoUrl && (
        <Card>
          <CardHeader
            title={
              <span className="inline-flex items-center gap-2">
                <Webhook className="size-4" /> Git Push Deployments
              </span>
            }
            description={`Add a GitHub webhook (content type application/json, “Just the push event”) to deploy on every push to ${p.branch}.`}
          />
          <div className="space-y-4 p-5">
            <div>
              <Label>Payload URL</Label>
              <div className="flex items-center gap-2 rounded-md border border-line bg-subtle px-3 py-2">
                <span className="min-w-0 flex-1 truncate font-mono text-[13px]">{webhook}</span>
                <CopyButton value={webhook} />
              </div>
            </div>
            <div>
              <Label>Secret</Label>
              <div className="flex items-center gap-2 rounded-md border border-line bg-subtle px-3 py-2">
                <span className="min-w-0 flex-1 truncate font-mono text-[13px]">{p.webhookSecret}</span>
                <CopyButton value={p.webhookSecret ?? ""} />
              </div>
            </div>
            <p className="text-xs text-muted">The webhook must reach the API from the internet (e.g. a tunnel or a public VPS) for GitHub to deliver it.</p>
          </div>
        </Card>
      )}

      <Card className="border-err/40">
        <CardHeader title="Delete Project" description="Permanently remove this project and all of its deployments." />
        <div className="flex justify-end border-t border-err/20 bg-err-bg/40 px-5 py-3">
          <Button variant="danger" size="sm" icon={<Trash2 className="size-4" />} onClick={del}>
            Delete
          </Button>
        </div>
      </Card>
    </div>
  );
}

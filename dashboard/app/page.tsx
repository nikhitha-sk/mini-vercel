"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  ArrowUpRight,
  Box,
  CheckCircle2,
  CheckSquare,
  Clock3,
  Copy,
  ExternalLink,
  FileArchive,
  Github,
  Globe2,
  LayoutGrid,
  List,
  Plus,
  Rocket,
  Search,
  Settings,
  Timer,
  Trash2,
  X,
} from "lucide-react";
import { AppShell, Container } from "@/components/shell";
import { Avatar, Button, ButtonLink, Card, Checkbox, EmptyState, ErrorBanner, Input, Kbd, Menu, Pill, Select, Skeleton, StatusBadge, StatusDot, type MenuItem } from "@/components/ui";
import { useFeedback } from "@/components/feedback";
import { useDeleteProjects } from "@/components/project-actions";
import { api, isActive, type Project, type RecentDeployment, type Stats } from "@/lib/api";
import { useResource, useTick } from "@/lib/hooks";
import { cn, duration, frameworkName, hostOf, repoPath, timeAgo } from "@/lib/format";

type View = "grid" | "list";
type Sort = "updated" | "deployed" | "name";

function usePref<T extends string>(key: string, initial: T): [T, (v: T) => void] {
  const [v, setV] = useState<T>(initial);
  useEffect(() => {
    try {
      const s = localStorage.getItem(key);
      if (s) setV(s as T);
    } catch {}
  }, [key]);
  return [
    v,
    (next: T) => {
      setV(next);
      try {
        localStorage.setItem(key, next);
      } catch {}
    },
  ];
}

export default function OverviewPage() {
  useTick();
  const router = useRouter();
  const deleteProjects = useDeleteProjects();
  const [q, setQ] = useState("");
  const [view, setView] = usePref<View>("mv_view", "grid");
  const [sort, setSort] = usePref<Sort>("mv_sort", "updated");
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const searchRef = useRef<HTMLInputElement>(null);

  const projects = useResource(() => api.projects(), [], {
    pollMs: 2500,
    pollWhile: (ps) => ps.some((p) => p.latestDeployment && isActive(p.latestDeployment.status)),
  });
  const stats = useResource(() => api.stats(), []);
  const recent = useResource(() => api.recent(), [], { pollMs: 3000, pollWhile: (ds) => ds.some((d) => isActive(d.status)) });

  // "/" focuses search, Escape leaves selection mode.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      const typing = t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable;
      if (e.key === "/" && !typing) {
        e.preventDefault();
        searchRef.current?.focus();
      } else if (e.key === "Escape" && selecting && !document.querySelector('[role="dialog"]')) {
        exitSelect();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selecting]);

  const filtered = useMemo(() => {
    let list = [...(projects.data ?? [])];
    if (q) {
      const s = q.toLowerCase();
      list = list.filter((p) => p.name.toLowerCase().includes(s) || p.repoUrl.toLowerCase().includes(s) || p.slug.includes(s));
    }
    const ts = (iso?: string | null) => (iso ? new Date(iso).getTime() : 0);
    if (sort === "name") list.sort((a, b) => a.name.localeCompare(b.name));
    else if (sort === "deployed") list.sort((a, b) => ts(b.latestDeployment?.createdAt) - ts(a.latestDeployment?.createdAt));
    else list.sort((a, b) => ts(b.updatedAt) - ts(a.updatedAt));
    return list;
  }, [projects.data, q, sort]);

  const exitSelect = () => {
    setSelecting(false);
    setSelected(new Set());
  };
  const toggle = (id: string) =>
    setSelected((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  const allVisibleSelected = filtered.length > 0 && filtered.every((p) => selected.has(p.id));

  const remove = async (ps: Project[]) => {
    const gone = await deleteProjects(ps);
    if (gone.length === 0) return;
    projects.setData((cur) => (cur ? cur.filter((p) => !gone.includes(p.id)) : cur));
    setSelected((s) => new Set([...s].filter((id) => !gone.includes(id))));
    if (gone.length === ps.length && ps.length > 1) exitSelect();
    stats.reload();
    recent.reload();
  };

  const count = projects.data?.length ?? 0;
  const selectedProjects = (projects.data ?? []).filter((p) => selected.has(p.id));

  return (
    <AppShell>
      <Container>
        <div className="mb-6 flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight sm:text-[28px]">Projects</h1>
            <p className="mt-1 text-sm text-muted">
              {projects.data ? `${count} project${count === 1 ? "" : "s"} · builds run in isolated containers` : "Loading your projects…"}
            </p>
          </div>
        </div>

        <StatsRow stats={stats.data} />

        <div className="mt-8 flex flex-col gap-3 md:flex-row md:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-faint" />
            <Input
              ref={searchRef}
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Search projects…"
              className="pl-9 pr-10"
              aria-label="Search projects"
            />
            {q ? (
              <button
                onClick={() => setQ("")}
                aria-label="Clear search"
                className="absolute right-2 top-1/2 flex size-6 -translate-y-1/2 cursor-pointer items-center justify-center rounded text-faint hover:bg-hover hover:text-fg"
              >
                <X className="size-3.5" />
              </button>
            ) : (
              <Kbd className="pointer-events-none absolute right-3 top-1/2 hidden -translate-y-1/2 sm:inline-flex">/</Kbd>
            )}
          </div>
          <div className="flex items-center gap-2">
            <div className="min-w-0 flex-1 md:w-44 md:flex-none">
              <Select value={sort} onChange={(e) => setSort(e.target.value as Sort)} aria-label="Sort projects">
                <option value="updated">Sort by activity</option>
                <option value="deployed">Sort by last deploy</option>
                <option value="name">Sort by name</option>
              </Select>
            </div>
            <div className="flex h-10 items-center rounded-md border border-line bg-elevated p-1" role="group" aria-label="Layout">
              {([
                ["grid", <LayoutGrid key="g" className="size-4" />, "Grid view"],
                ["list", <List key="l" className="size-4" />, "List view"],
              ] as const).map(([v, icon, label]) => (
                <button
                  key={v}
                  onClick={() => setView(v)}
                  aria-label={label}
                  aria-pressed={view === v}
                  title={label}
                  className={cn(
                    "flex h-full w-8 cursor-pointer items-center justify-center rounded transition-colors",
                    view === v ? "bg-hover text-fg" : "text-faint hover:text-fg",
                  )}
                >
                  {icon}
                </button>
              ))}
            </div>
            {count > 0 && (
              <Button
                variant={selecting ? "primary" : "secondary"}
                icon={selecting ? <X className="size-4" /> : <CheckSquare className="size-4" />}
                onClick={() => (selecting ? exitSelect() : setSelecting(true))}
              >
                {selecting ? "Done" : "Select"}
              </Button>
            )}
            <div className="hidden md:block">
              <ButtonLink href="/new" variant="primary" icon={<Plus className="size-4" />}>
                Add New
              </ButtonLink>
            </div>
          </div>
        </div>

        {selecting && (
          <div className="animate-fade-in mt-3 flex items-center gap-3 text-[13px] text-muted">
            <Checkbox
              checked={allVisibleSelected}
              onChange={(v) => setSelected(v ? new Set([...selected, ...filtered.map((p) => p.id)]) : new Set())}
              label="Select all"
            />
            <span>{allVisibleSelected ? "Deselect all" : "Select all"}</span>
            <span className="text-faint">·</span>
            <span>Click projects to select them, then delete them together.</span>
          </div>
        )}

        <div className="mt-6 grid grid-cols-1 gap-8 lg:grid-cols-[minmax(0,1fr)_320px]">
          <section aria-label="Projects">
            {projects.error && <ErrorBanner message={projects.error} onRetry={projects.reload} />}
            {projects.loading && !projects.data && (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {[0, 1, 2, 3].map((i) => (
                  <Card key={i} className="p-5">
                    <Skeleton className="h-8 w-8 rounded-full" />
                    <Skeleton className="mt-4 h-4 w-2/3" />
                    <Skeleton className="mt-2 h-3 w-1/2" />
                    <Skeleton className="mt-6 h-3 w-full" />
                  </Card>
                ))}
              </div>
            )}
            {projects.data && projects.data.length === 0 && (
              <Card>
                <EmptyState
                  icon={<Rocket className="size-5" />}
                  title="Deploy your first project"
                  description="Import a Git repository or upload a zip. We'll build it in an isolated container and give you a live URL in about a minute."
                  action={
                    <ButtonLink href="/new" variant="primary" icon={<Plus className="size-4" />}>
                      New Project
                    </ButtonLink>
                  }
                />
              </Card>
            )}
            {projects.data && projects.data.length > 0 && filtered.length === 0 && (
              <Card>
                <EmptyState
                  icon={<Search className="size-5" />}
                  title="No matching projects"
                  description={`Nothing matches “${q}”.`}
                  action={
                    <Button variant="secondary" size="sm" onClick={() => setQ("")}>
                      Clear search
                    </Button>
                  }
                />
              </Card>
            )}

            {view === "grid" ? (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {filtered.map((p) => (
                  <ProjectCard
                    key={p.id}
                    p={p}
                    selecting={selecting}
                    selected={selected.has(p.id)}
                    onToggle={() => toggle(p.id)}
                    onDelete={() => remove([p])}
                    onOpen={(path) => router.push(path)}
                  />
                ))}
              </div>
            ) : (
              filtered.length > 0 && (
                <Card className="divide-y divide-line">
                  {filtered.map((p) => (
                    <ProjectRow
                      key={p.id}
                      p={p}
                      selecting={selecting}
                      selected={selected.has(p.id)}
                      onToggle={() => toggle(p.id)}
                      onDelete={() => remove([p])}
                    />
                  ))}
                </Card>
              )
            )}
          </section>

          <aside aria-label="Recent activity">
            <h2 className="mb-3 text-sm font-semibold">Recent Deployments</h2>
            <Card className="overflow-hidden">
              {recent.loading && !recent.data && (
                <div className="space-y-4 p-4">
                  {[0, 1, 2].map((i) => (
                    <Skeleton key={i} className="h-9" />
                  ))}
                </div>
              )}
              {recent.data && recent.data.length === 0 && <p className="p-4 text-[13px] text-muted">No deployments yet.</p>}
              <ul className="divide-y divide-line">
                {recent.data?.slice(0, 8).map((d) => (
                  <RecentRow key={d.id} d={d} />
                ))}
              </ul>
              {recent.data && recent.data.length > 0 && (
                <Link href="/deployments" className="block border-t border-line px-4 py-3 text-center text-[13px] text-muted hover:bg-hover hover:text-fg">
                  View all deployments
                </Link>
              )}
            </Card>
          </aside>
        </div>
      </Container>

      {selecting && (
        <div className="pointer-events-none fixed inset-x-0 bottom-6 z-40 flex justify-center px-4">
          <div className="animate-pop-in pointer-events-auto flex items-center gap-3 rounded-full border border-line bg-elevated py-2 pl-5 pr-2 shadow-pop">
            <span className="text-[13px] font-medium tabular-nums">
              {selected.size} selected
            </span>
            <span className="h-5 w-px bg-line" />
            <Button variant="ghost" size="sm" onClick={() => setSelected(new Set())} disabled={selected.size === 0}>
              Clear
            </Button>
            <Button
              variant="danger"
              size="sm"
              icon={<Trash2 className="size-4" />}
              disabled={selected.size === 0}
              onClick={() => remove(selectedProjects)}
              className="rounded-full"
            >
              Delete{selected.size > 0 ? ` ${selected.size}` : ""}
            </Button>
          </div>
        </div>
      )}
    </AppShell>
  );
}

function StatsRow({ stats }: { stats: Stats | null }) {
  const success = stats && stats.ready + stats.failed > 0 ? Math.round((stats.ready / (stats.ready + stats.failed)) * 100) : null;
  const items = [
    { label: "Projects", value: stats?.projects ?? "–", icon: <Box className="size-4" />, tint: "text-info bg-info-bg" },
    { label: "Deployments (30d)", value: stats?.deployments ?? "–", icon: <Activity className="size-4" />, tint: "text-warn bg-warn-bg" },
    { label: "Success rate", value: success == null ? "–" : `${success}%`, icon: <CheckCircle2 className="size-4" />, tint: "text-ok bg-ok-bg" },
    {
      label: "Avg. build time",
      value: stats?.avgBuildSeconds ? duration(Math.round(stats.avgBuildSeconds * 1000)) : "–",
      icon: <Timer className="size-4" />,
      tint: "text-muted bg-subtle",
    },
  ];
  return (
    <div className="grid grid-cols-2 overflow-hidden rounded-xl border border-line bg-line shadow-card lg:grid-cols-4" style={{ gap: 1 }}>
      {items.map((i) => (
        <div key={i.label} className="flex items-center gap-3.5 bg-elevated p-5">
          <div className={cn("hidden size-9 shrink-0 items-center justify-center rounded-lg sm:flex", i.tint)}>{i.icon}</div>
          <div className="min-w-0">
            <div className="truncate text-[13px] text-muted">{i.label}</div>
            <div className="mt-0.5 text-2xl font-semibold tracking-tight tabular-nums">{stats ? i.value : <Skeleton className="h-7 w-16" />}</div>
          </div>
        </div>
      ))}
    </div>
  );
}

function projectMenu(p: Project, onDelete: () => void, toast: ReturnType<typeof useFeedback>["toast"]): MenuItem[] {
  const url = p.liveUrl || p.productionUrl;
  return [
    { label: "Visit site", icon: <ExternalLink className="size-4" />, href: url, external: true, hidden: !p.productionDeployment },
    { label: "View deployments", icon: <Rocket className="size-4" />, href: `/project?id=${p.id}&tab=deployments` },
    { label: "Settings", icon: <Settings className="size-4" />, href: `/project?id=${p.id}&tab=settings` },
    {
      label: "Copy URL",
      icon: <Copy className="size-4" />,
      hidden: !p.productionDeployment,
      onSelect: () => {
        navigator.clipboard?.writeText(url);
        toast("success", "URL copied", hostOf(url));
      },
    },
    { label: "Delete project", icon: <Trash2 className="size-4" />, onSelect: onDelete, danger: true, separated: true },
  ];
}

function frameworkOf(p: Project) {
  const f = p.latestDeployment?.framework || (p.framework !== "auto" ? p.framework : "");
  return f ? frameworkName(f) : "";
}

function ProjectCard({
  p,
  selecting,
  selected,
  onToggle,
  onDelete,
  onOpen,
}: {
  p: Project;
  selecting: boolean;
  selected: boolean;
  onToggle: () => void;
  onDelete: () => void;
  onOpen: (path: string) => void;
}) {
  const { toast } = useFeedback();
  const d = p.latestDeployment;
  const prod = p.productionDeployment;
  const fw = frameworkOf(p);
  const href = `/project?id=${p.id}`;
  return (
    <Card
      className={cn(
        "group relative flex flex-col p-5 transition-all duration-150",
        selected ? "border-fg ring-1 ring-fg" : "hover:border-line-strong hover:shadow-pop",
      )}
    >
      <div className="flex items-start gap-3">
        <Avatar name={p.name} className="size-9" />
        <div className="min-w-0 flex-1">
          {selecting ? (
            <button
              onClick={onToggle}
              className="block w-full cursor-pointer truncate text-left text-[15px] font-semibold tracking-tight after:absolute after:inset-0"
            >
              {p.name}
            </button>
          ) : (
            <Link href={href} className="block truncate text-[15px] font-semibold tracking-tight after:absolute after:inset-0">
              {p.name}
            </Link>
          )}
          {prod ? (
            <a
              href={p.liveUrl || p.productionUrl}
              target="_blank"
              rel="noreferrer"
              className="relative z-10 mt-0.5 block truncate text-[13px] text-muted hover:text-fg hover:underline"
            >
              {hostOf(p.liveUrl || p.productionUrl)}
            </a>
          ) : (
            <span className="mt-0.5 block text-[13px] text-faint">No production deployment</span>
          )}
        </div>
        {selecting ? (
          <Checkbox checked={selected} onChange={onToggle} label={`Select ${p.name}`} className="relative z-10 mt-1" />
        ) : (
          <div className="flex items-center gap-1">
            {prod && (
              <a
                href={p.liveUrl || p.productionUrl}
                target="_blank"
                rel="noreferrer"
                aria-label={`Visit ${p.name}`}
                title="Visit"
                className="relative z-10 hidden size-8 items-center justify-center rounded-md text-muted opacity-0 transition-opacity hover:bg-hover hover:text-fg group-hover:opacity-100 sm:flex"
              >
                <ArrowUpRight className="size-4" />
              </a>
            )}
            <Menu items={projectMenu(p, onDelete, toast)} label={`Actions for ${p.name}`} />
          </div>
        )}
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-1.5">
        {p.repoUrl ? (
          <span className="inline-flex max-w-full items-center gap-1.5 rounded-full bg-subtle px-2.5 py-1 text-xs font-medium text-muted">
            <Github className="size-3.5 shrink-0" />
            <span className="truncate">{repoPath(p.repoUrl)}</span>
          </span>
        ) : (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-subtle px-2.5 py-1 text-xs font-medium text-muted">
            <FileArchive className="size-3.5 shrink-0" /> Zip upload
          </span>
        )}
        {fw && <Pill>{fw}</Pill>}
        {p.liveStatus === "live" && (
          <Pill tone="ok">
            <Globe2 className="size-3" /> Live
          </Pill>
        )}
      </div>

      {p.publicStatus === "live" && p.publicUrl && (
        <a
          href={p.publicUrl}
          target="_blank"
          rel="noreferrer"
          className="relative z-10 mt-3 inline-flex max-w-full items-center gap-1.5 self-start text-[13px] font-medium text-link hover:underline"
        >
          <Globe2 className="size-3.5 shrink-0" />
          <span className="truncate">{hostOf(p.publicUrl)}</span>
        </a>
      )}

      <div className="mt-4 flex-1 border-t border-line pt-4 text-[13px]">
        {d ? (
          <>
            <p className="truncate text-fg">{d.commitMessage || (d.source === "zip" ? "Uploaded archive" : "Deployment")}</p>
            <div className="mt-1.5 flex items-center gap-2 text-muted">
              <StatusBadge status={d.status} size="sm" />
              <span className="text-faint">·</span>
              <span>{timeAgo(d.createdAt)}</span>
              {d.branch && d.source === "git" && (
                <>
                  <span className="text-faint">on</span>
                  <span className="truncate font-mono text-xs">{d.branch}</span>
                </>
              )}
            </div>
          </>
        ) : (
          <div className="flex items-center justify-between gap-2">
            <p className="flex items-center gap-1.5 text-muted">
              <Clock3 className="size-3.5" /> Not deployed yet
            </p>
            {!selecting && (
              <button
                onClick={() => onOpen(href)}
                className="relative z-10 cursor-pointer text-[13px] font-medium text-link hover:underline"
              >
                Deploy now
              </button>
            )}
          </div>
        )}
      </div>
    </Card>
  );
}

function ProjectRow({
  p,
  selecting,
  selected,
  onToggle,
  onDelete,
}: {
  p: Project;
  selecting: boolean;
  selected: boolean;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const { toast } = useFeedback();
  const d = p.latestDeployment;
  const prod = p.productionDeployment;
  const fw = frameworkOf(p);
  return (
    <div className={cn("group relative flex items-center gap-4 px-4 py-3.5 transition-colors first:rounded-t-xl last:rounded-b-xl", selected ? "bg-hover" : "hover:bg-hover")}>
      {selecting && <Checkbox checked={selected} onChange={onToggle} label={`Select ${p.name}`} className="relative z-10" />}
      <Avatar name={p.name} />
      <div className="min-w-0 flex-1">
        {selecting ? (
          <button onClick={onToggle} className="block w-full cursor-pointer truncate text-left text-sm font-semibold after:absolute after:inset-0">
            {p.name}
          </button>
        ) : (
          <Link href={`/project?id=${p.id}`} className="block truncate text-sm font-semibold after:absolute after:inset-0">
            {p.name}
          </Link>
        )}
        <p className="truncate text-[13px] text-muted">{prod ? hostOf(p.liveUrl || p.productionUrl) : "No production deployment"}</p>
      </div>
      <div className="hidden w-48 min-w-0 md:block">
        <p className="flex items-center gap-1.5 truncate text-[13px] text-muted">
          {p.repoUrl ? <Github className="size-3.5 shrink-0" /> : <FileArchive className="size-3.5 shrink-0" />}
          <span className="truncate">{p.repoUrl ? repoPath(p.repoUrl) : "Zip upload"}</span>
        </p>
        {fw && <p className="mt-0.5 truncate text-xs text-faint">{fw}</p>}
      </div>
      <div className="hidden w-32 shrink-0 sm:block">
        {d ? (
          <>
            <StatusBadge status={d.status} size="sm" />
            <p className="mt-0.5 text-xs text-faint">{timeAgo(d.createdAt)}</p>
          </>
        ) : (
          <span className="text-xs text-faint">Not deployed</span>
        )}
      </div>
      {d && <StatusDot status={d.status} className="sm:hidden" />}
      {!selecting && <Menu items={projectMenu(p, onDelete, toast)} label={`Actions for ${p.name}`} />}
    </div>
  );
}

function RecentRow({ d }: { d: RecentDeployment }) {
  return (
    <li>
      <Link href={`/deployment?id=${d.id}`} className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-hover">
        <StatusDot status={d.status} />
        <div className="min-w-0 flex-1">
          <p className="truncate text-[13px] font-medium">{d.projectName}</p>
          <p className="truncate text-xs text-muted">{d.commitMessage || hostOf(d.url).split(".")[0]}</p>
        </div>
        <span className="shrink-0 text-xs text-faint">{timeAgo(d.createdAt)}</span>
      </Link>
    </li>
  );
}

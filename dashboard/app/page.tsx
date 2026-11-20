"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { Activity, ArrowUpRight, Box, CheckCircle2, Clock3, Github, Globe2, Plus, Rocket, Search, Timer } from "lucide-react";
import { AppShell, Container } from "@/components/shell";
import { Avatar, ButtonLink, Card, EmptyState, ErrorBanner, Input, Skeleton, StatusDot } from "@/components/ui";
import { api, isActive, type Project, type RecentDeployment, type Stats } from "@/lib/api";
import { useResource, useTick } from "@/lib/hooks";
import { duration, hostOf, repoPath, timeAgo } from "@/lib/format";

export default function OverviewPage() {
  useTick();
  const [q, setQ] = useState("");
  const projects = useResource(() => api.projects(), [], {
    pollMs: 2500,
    pollWhile: (ps) => ps.some((p) => p.latestDeployment && isActive(p.latestDeployment.status)),
  });
  const stats = useResource(() => api.stats(), []);
  const recent = useResource(() => api.recent(), [], { pollMs: 3000, pollWhile: (ds) => ds.some((d) => isActive(d.status)) });

  const filtered = useMemo(() => {
    const list = projects.data ?? [];
    if (!q) return list;
    const s = q.toLowerCase();
    return list.filter((p) => p.name.toLowerCase().includes(s) || p.repoUrl.toLowerCase().includes(s) || p.slug.includes(s));
  }, [projects.data, q]);

  return (
    <AppShell>
      <Container>
        <StatsRow stats={stats.data} />

        <div className="mt-8 flex flex-col gap-3 sm:flex-row sm:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-faint" />
            <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search projects…" className="pl-9" aria-label="Search projects" />
          </div>
          <ButtonLink href="/new" variant="primary" icon={<Plus className="size-4" />}>
            Add New Project
          </ButtonLink>
        </div>

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
                <EmptyState icon={<Search className="size-5" />} title="No matching projects" description={`Nothing matches “${q}”.`} />
              </Card>
            )}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {filtered.map((p) => (
                <ProjectCard key={p.id} p={p} />
              ))}
            </div>
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
    </AppShell>
  );
}

function StatsRow({ stats }: { stats: Stats | null }) {
  const success = stats && stats.ready + stats.failed > 0 ? Math.round((stats.ready / (stats.ready + stats.failed)) * 100) : null;
  const items = [
    { label: "Projects", value: stats?.projects ?? "–", icon: <Box className="size-4" /> },
    { label: "Deployments (30d)", value: stats?.deployments ?? "–", icon: <Activity className="size-4" /> },
    { label: "Success rate", value: success == null ? "–" : `${success}%`, icon: <CheckCircle2 className="size-4" /> },
    { label: "Avg. build time", value: stats?.avgBuildSeconds ? duration(Math.round(stats.avgBuildSeconds * 1000)) : "–", icon: <Timer className="size-4" /> },
  ];
  return (
    <div className="grid grid-cols-2 overflow-hidden rounded-xl border border-line bg-line shadow-card lg:grid-cols-4" style={{ gap: 1 }}>
      {items.map((i) => (
        <div key={i.label} className="bg-elevated p-5">
          <div className="flex items-center gap-2 text-[13px] text-muted">
            {i.icon}
            {i.label}
          </div>
          <div className="mt-2 text-2xl font-semibold tracking-tight tabular-nums">{stats ? i.value : <Skeleton className="h-7 w-16" />}</div>
        </div>
      ))}
    </div>
  );
}

function ProjectCard({ p }: { p: Project }) {
  const d = p.latestDeployment;
  const prod = p.productionDeployment;
  return (
    <Card className="group relative flex flex-col p-5 transition-all hover:border-line-strong hover:shadow-pop">
      <div className="flex items-start gap-3">
        <Avatar name={p.name} />
        <div className="min-w-0 flex-1">
          <Link href={`/project?id=${p.id}`} className="block truncate text-[15px] font-semibold tracking-tight after:absolute after:inset-0">
            {p.name}
          </Link>
          {prod ? (
            <a
              href={p.productionUrl}
              target="_blank"
              rel="noreferrer"
              className="relative z-10 mt-0.5 block truncate text-[13px] text-muted hover:text-fg hover:underline"
            >
              {hostOf(p.productionUrl)}
            </a>
          ) : (
            <span className="mt-0.5 block text-[13px] text-faint">No production deployment</span>
          )}
        </div>
        {prod && (
          <a
            href={p.productionUrl}
            target="_blank"
            rel="noreferrer"
            aria-label={`Visit ${p.name}`}
            className="relative z-10 flex size-8 items-center justify-center rounded-full border border-line text-muted opacity-0 transition-opacity hover:text-fg group-hover:opacity-100"
          >
            <ArrowUpRight className="size-4" />
          </a>
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

      {p.repoUrl && (
        <div className="mt-4 inline-flex max-w-full items-center gap-1.5 self-start rounded-full bg-subtle px-2.5 py-1 text-xs font-medium text-muted">
          <Github className="size-3.5 shrink-0" />
          <span className="truncate">{repoPath(p.repoUrl)}</span>
        </div>
      )}

      <div className="mt-4 flex-1 text-[13px]">
        {d ? (
          <>
            <p className="truncate text-fg">{d.commitMessage || (d.source === "zip" ? "Uploaded archive" : "Deployment")}</p>
            <div className="mt-1.5 flex items-center gap-2 text-muted">
              <StatusDot status={d.status} />
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
          <p className="flex items-center gap-1.5 text-muted">
            <Clock3 className="size-3.5" /> Not deployed yet
          </p>
        )}
      </div>
    </Card>
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

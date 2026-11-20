"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";
import { Check, ExternalLink, FileArchive, GitBranch, GitCommitHorizontal, RefreshCw, Undo2, XCircle } from "lucide-react";
import { AppShell, Container } from "@/components/shell";
import { Button, ButtonLink, Card, ErrorBanner, Kv, Pill, Skeleton, StatusBadge } from "@/components/ui";
import { CopyButton, useFeedback } from "@/components/feedback";
import { LogViewer } from "@/components/log-viewer";
import { api, isActive, type Deployment, type Status } from "@/lib/api";
import { useResource, useTick } from "@/lib/hooks";
import { bytes, cn, duration, hostOf, shortSha, timeAgo } from "@/lib/format";

export default function DeploymentPage() {
  return (
    <Suspense>
      <DeploymentView />
    </Suspense>
  );
}

function DeploymentView() {
  useTick(1000);
  const id = useSearchParams().get("id") ?? "";
  const router = useRouter();
  const { toast, confirm } = useFeedback();
  const res = useResource(() => api.deployment(id), [id]);
  const [d, setD] = useState<Deployment | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    if (res.data) setD(res.data.deployment);
  }, [res.data]);

  const project = res.data?.project;

  const act = async (name: string, fn: () => Promise<void>) => {
    setBusy(name);
    try {
      await fn();
    } catch (e) {
      toast("error", "Action failed", e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  const promote = () =>
    act("promote", async () => {
      if (!d || !project) return;
      const ok = await confirm({
        title: "Promote to Production",
        description: (
          <>
            <span className="font-mono text-fg">{hostOf(project.productionUrl)}</span> will serve this deployment immediately. This is how
            instant rollback works: a single pointer swap.
          </>
        ),
        confirmLabel: "Promote",
      });
      if (!ok) return;
      const r = await api.promote(d.id);
      setD({ ...d, isProduction: true });
      toast("success", "Production updated", `Switched in ${r.tookMs.toFixed(1)} ms`);
    });

  const redeploy = () =>
    act("redeploy", async () => {
      if (!d) return;
      const nd = await api.redeploy(d.id);
      toast("success", "Redeploy queued");
      router.push(`/deployment?id=${nd.id}`);
    });

  const cancel = () =>
    act("cancel", async () => {
      if (!d) return;
      const nd = await api.cancel(d.id);
      setD(nd);
      toast("info", "Deployment canceled");
    });

  const live = d ? isActive(d.status) : false;
  const elapsed = d && live && d.startedAt ? Date.now() - new Date(d.startedAt).getTime() : d?.durationMs ?? null;

  return (
    <AppShell
      crumbs={[
        { label: project?.name ?? "Project", href: project ? `/project?id=${project.id}` : undefined },
        { label: <span className="font-mono text-[13px]">{id.replace("_", "-")}</span> },
      ]}
      tabs={
        project
          ? [
              { label: "Deployment", href: `/deployment?id=${id}`, active: true },
              { label: "All Deployments", href: `/project?id=${project.id}&tab=deployments`, active: false },
            ]
          : []
      }
    >
      <Container>
        {res.error && <ErrorBanner message={res.error} onRetry={res.reload} />}
        {!d && !res.error && (
          <>
            <Skeleton className="h-8 w-72" />
            <Skeleton className="mt-6 h-48 w-full" />
            <Skeleton className="mt-6 h-96 w-full" />
          </>
        )}
        {d && project && (
          <div className="space-y-6">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h1 className="text-2xl font-semibold tracking-tight">Deployment Details</h1>
                  {d.isProduction && <Pill tone="info">Production · Current</Pill>}
                  {!d.isProduction && d.status === "READY" && <Pill>Preview</Pill>}
                </div>
                <p className="mt-1 font-mono text-[13px] text-muted">{d.id}</p>
              </div>
              <div className="flex flex-wrap gap-2">
                {live && (
                  <Button variant="secondary" icon={<XCircle className="size-4" />} onClick={cancel} loading={busy === "cancel"}>
                    Cancel
                  </Button>
                )}
                {!live && (
                  <Button variant="secondary" icon={<RefreshCw className="size-4" />} onClick={redeploy} loading={busy === "redeploy"}>
                    Redeploy
                  </Button>
                )}
                {d.status === "READY" && !d.isProduction && (
                  <Button variant="secondary" icon={<Undo2 className="size-4" />} onClick={promote} loading={busy === "promote"}>
                    Promote to Production
                  </Button>
                )}
                {d.status === "READY" && (
                  <ButtonLink href={d.url} external variant="primary" icon={<ExternalLink className="size-4" />}>
                    Visit
                  </ButtonLink>
                )}
              </div>
            </div>

            <Card>
              <div className="grid grid-cols-1 gap-6 p-5 sm:grid-cols-2 lg:grid-cols-4">
                <Kv label="Status">
                  <StatusBadge status={d.status} />
                </Kv>
                <Kv label="Duration">
                  <span className="tabular-nums">{duration(elapsed)}</span>
                  {d.attempt > 1 && <span className="text-muted"> · attempt {d.attempt}</span>}
                </Kv>
                <Kv label="Created">
                  {timeAgo(d.createdAt)} <span className="text-muted">· {d.trigger}</span>
                </Kv>
                <Kv label="Output">{d.fileCount ? `${d.fileCount} files · ${bytes(d.sizeBytes)}` : "–"}</Kv>
                <div className="min-w-0 sm:col-span-2">
                  <div className="mb-1 text-xs font-medium text-faint">Deployment URL</div>
                  {d.status === "READY" ? (
                    <div className="flex items-center gap-1">
                      <a href={d.url} target="_blank" rel="noreferrer" className="truncate font-mono text-[13px] font-medium hover:underline">
                        {hostOf(d.url)}
                      </a>
                      <CopyButton value={d.url} />
                    </div>
                  ) : (
                    <span className="font-mono text-[13px] text-faint">{hostOf(d.url)}</span>
                  )}
                </div>
                <div className="min-w-0 sm:col-span-2">
                  <div className="mb-1 text-xs font-medium text-faint">Source</div>
                  {d.source === "zip" ? (
                    <div className="flex items-center gap-1.5 text-sm">
                      <FileArchive className="size-3.5 text-muted" /> Zip upload
                    </div>
                  ) : (
                    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                      <span className="flex items-center gap-1.5">
                        <GitBranch className="size-3.5 text-muted" />
                        <span className="font-mono text-[13px]">{d.branch}</span>
                      </span>
                      <span className="flex min-w-0 items-center gap-1.5">
                        <GitCommitHorizontal className="size-3.5 shrink-0 text-muted" />
                        {d.commitSha && project.repoUrl.includes("github.com") ? (
                          <a href={`${project.repoUrl}/commit/${d.commitSha}`} target="_blank" rel="noreferrer" className="font-mono text-[13px] hover:underline">
                            {shortSha(d.commitSha)}
                          </a>
                        ) : (
                          <span className="font-mono text-[13px] text-muted">{shortSha(d.commitSha) || "–"}</span>
                        )}
                        <span className="truncate">{d.commitMessage}</span>
                      </span>
                    </div>
                  )}
                </div>
              </div>
              <Pipeline status={d.status} error={d.error} />
            </Card>

            {d.status === "FAILED" && d.error && (
              <div className="rounded-xl border border-err/30 bg-err-bg px-5 py-4">
                <p className="text-sm font-semibold text-err">Build failed</p>
                <p className="mt-1 font-mono text-[13px] text-err">{d.error}</p>
              </div>
            )}

            <LogViewer deploymentId={d.id} live={live} onStatus={(nd) => setD((cur) => ({ ...nd, isProduction: nd.isProduction || (cur?.isProduction ?? false) }))} />

            <Card>
              <div className="grid grid-cols-1 gap-6 p-5 sm:grid-cols-3">
                <Kv label="Install command" mono>
                  {d.installCommand || "auto-detect"}
                </Kv>
                <Kv label="Build command" mono>
                  {d.buildCommand || "none"}
                </Kv>
                <Kv label="Output directory" mono>
                  {d.rootDirectory ? `${d.rootDirectory}/` : ""}
                  {d.outputDirectory}
                </Kv>
              </div>
            </Card>

            <p className="text-center text-xs text-faint">
              Deployments are immutable. <Link href={`/project?id=${project.id}&tab=deployments`} className="underline underline-offset-2 hover:text-fg">Back to all deployments</Link>
            </p>
          </div>
        )}
      </Container>
    </AppShell>
  );
}

const STEPS: { key: Status; label: string }[] = [
  { key: "QUEUED", label: "Queued" },
  { key: "BUILDING", label: "Building" },
  { key: "UPLOADING", label: "Uploading" },
  { key: "READY", label: "Ready" },
];

function Pipeline({ status, error }: { status: Status; error: string }) {
  const failed = status === "FAILED" || status === "CANCELED";
  const idx = STEPS.findIndex((s) => s.key === status);
  return (
    <div className="flex items-center gap-2 overflow-x-auto border-t border-line px-5 py-4">
      {STEPS.map((s, i) => {
        const done = !failed && (idx > i || status === "READY");
        const current = !failed && idx === i && status !== "READY";
        return (
          <div key={s.key} className="flex items-center gap-2">
            <div
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-full border text-[11px] font-semibold",
                done && "border-ok bg-ok text-white",
                current && "animate-pulse-dot border-warn text-warn",
                !done && !current && "border-line text-faint",
              )}
            >
              {done ? <Check className="size-3.5" /> : i + 1}
            </div>
            <span className={cn("whitespace-nowrap text-[13px]", done || current ? "text-fg" : "text-faint")}>{s.label}</span>
            {i < STEPS.length - 1 && <div className={cn("h-px w-8 sm:w-16", done ? "bg-ok" : "bg-line")} />}
          </div>
        );
      })}
      {failed && (
        <span className="ml-2 inline-flex items-center gap-1.5 whitespace-nowrap text-[13px] font-medium text-err" title={error}>
          <XCircle className="size-4" /> {status === "FAILED" ? "Failed" : "Canceled"}
        </span>
      )}
    </div>
  );
}

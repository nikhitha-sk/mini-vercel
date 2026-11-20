"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { ExternalLink, FileArchive, GitBranch, GitCommitHorizontal, MoreHorizontal, RefreshCw, ScrollText, Undo2, XCircle } from "lucide-react";
import { api, isActive, type Deployment, type RecentDeployment } from "@/lib/api";
import { cn, duration, hostOf, shortSha, timeAgo } from "@/lib/format";
import { Pill, StatusBadge } from "./ui";
import { useFeedback } from "./feedback";

const triggerLabel: Record<Deployment["trigger"], string> = {
  manual: "Manual",
  api: "API",
  webhook: "Git push",
  redeploy: "Redeploy",
};

export function DeploymentList({
  deployments,
  onChanged,
  showProject,
}: {
  deployments: (Deployment | RecentDeployment)[];
  onChanged?: () => void;
  showProject?: boolean;
}) {
  return (
    <ul className="divide-y divide-line">
      {deployments.map((d) => (
        <DeploymentRow key={d.id} d={d} onChanged={onChanged} showProject={showProject} />
      ))}
    </ul>
  );
}

function DeploymentRow({ d, onChanged, showProject }: { d: Deployment | RecentDeployment; onChanged?: () => void; showProject?: boolean }) {
  const project = "projectName" in d ? d : null;
  const live = isActive(d.status);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    if (!live) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [live]);
  const dur = live && d.startedAt ? now - new Date(d.startedAt).getTime() : d.durationMs;

  return (
    <li className="group relative grid grid-cols-1 gap-3 px-5 py-4 transition-colors hover:bg-subtle/60 md:grid-cols-[minmax(0,1.3fr)_minmax(0,0.9fr)_minmax(0,1.4fr)_auto] md:items-center md:gap-6">
      <div className="min-w-0">
        <Link href={`/deployment?id=${d.id}`} className="block truncate font-mono text-[13px] font-medium text-fg after:absolute after:inset-0">
          {hostOf(d.url).split(".")[0]}
        </Link>
        <div className="mt-1 flex items-center gap-2 text-xs text-muted">
          {showProject && project ? <span className="truncate">{project.projectName}</span> : <span>{d.isProduction ? "Production" : "Preview"}</span>}
          {d.isProduction && (
            <Pill tone="info" className="h-[18px] px-1.5 text-[10px]">
              Current
            </Pill>
          )}
        </div>
      </div>

      <div className="flex items-center gap-3 md:block">
        <StatusBadge status={d.status} />
        <div className="text-xs text-muted md:mt-1">{d.status === "QUEUED" ? "Waiting for worker" : duration(dur)}</div>
      </div>

      <div className="min-w-0 text-[13px]">
        {d.source === "zip" ? (
          <div className="flex items-center gap-1.5 text-muted">
            <FileArchive className="size-3.5 shrink-0" />
            <span className="truncate">{d.commitMessage || "Uploaded archive"}</span>
          </div>
        ) : (
          <>
            <div className="flex items-center gap-1.5 text-muted">
              <GitBranch className="size-3.5 shrink-0" />
              <span className="truncate font-mono text-xs">{d.branch}</span>
            </div>
            <div className="mt-1 flex items-center gap-1.5 text-fg">
              <GitCommitHorizontal className="size-3.5 shrink-0 text-muted" />
              {d.commitSha && <span className="font-mono text-xs text-muted">{shortSha(d.commitSha)}</span>}
              <span className="truncate">{d.commitMessage || (live ? "Fetching commit…" : "–")}</span>
            </div>
          </>
        )}
      </div>

      <div className="flex items-center justify-between gap-3 md:justify-end">
        <div className="text-right text-xs text-muted">
          <div>{timeAgo(d.createdAt)}</div>
          <div className="mt-0.5 text-faint">{triggerLabel[d.trigger]}</div>
        </div>
        <ActionsMenu d={d} onChanged={onChanged} />
      </div>
    </li>
  );
}

export function ActionsMenu({ d, onChanged }: { d: Deployment; onChanged?: () => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const { toast, confirm } = useFeedback();

  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => ref.current && !ref.current.contains(e.target as Node) && setOpen(false);
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [open]);

  const run = async (fn: () => Promise<unknown>, ok: string) => {
    setOpen(false);
    try {
      await fn();
      toast("success", ok);
      onChanged?.();
    } catch (e) {
      toast("error", "Action failed", e instanceof Error ? e.message : String(e));
    }
  };

  const promote = async () => {
    setOpen(false);
    const yes = await confirm({
      title: d.isProduction ? "Already in production" : "Promote to Production",
      description: (
        <>
          The production domain will point at <span className="font-mono text-fg">{d.id}</span> immediately. No rebuild, no downtime.
          You can roll back to any other ready deployment at any time.
        </>
      ),
      confirmLabel: "Promote",
    });
    if (yes) {
      try {
        const r = await api.promote(d.id);
        toast("success", "Production updated", `${r.hostname} now serves ${d.id} (switched in ${r.tookMs.toFixed(1)} ms)`);
        onChanged?.();
      } catch (e) {
        toast("error", "Promote failed", e instanceof Error ? e.message : String(e));
      }
    }
  };

  return (
    <div ref={ref} className="relative z-10">
      <button
        aria-label="Deployment actions"
        onClick={() => setOpen((v) => !v)}
        className="flex size-8 cursor-pointer items-center justify-center rounded-md text-muted transition-colors hover:bg-hover hover:text-fg"
      >
        <MoreHorizontal className="size-4" />
      </button>
      {open && (
        <div className="animate-fade-in absolute right-0 top-9 z-30 w-56 overflow-hidden rounded-lg border border-line bg-elevated p-1 shadow-pop">
          <MenuLink href={`/deployment?id=${d.id}`} icon={<ScrollText className="size-4" />}>
            Inspect deployment
          </MenuLink>
          {d.status === "READY" && (
            <MenuLink href={d.url} external icon={<ExternalLink className="size-4" />}>
              Visit
            </MenuLink>
          )}
          <div className="my-1 h-px bg-line" />
          {d.status === "READY" && !d.isProduction && (
            <MenuItem icon={<Undo2 className="size-4" />} onClick={promote}>
              Promote / Instant Rollback
            </MenuItem>
          )}
          {!isActive(d.status) && (
            <MenuItem icon={<RefreshCw className="size-4" />} onClick={() => run(() => api.redeploy(d.id), "Redeploy queued")}>
              Redeploy
            </MenuItem>
          )}
          {isActive(d.status) && (
            <MenuItem danger icon={<XCircle className="size-4" />} onClick={() => run(() => api.cancel(d.id), "Deployment canceled")}>
              Cancel deployment
            </MenuItem>
          )}
        </div>
      )}
    </div>
  );
}

function MenuItem({ children, icon, onClick, danger }: { children: React.ReactNode; icon: React.ReactNode; onClick: () => void; danger?: boolean }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13px] transition-colors hover:bg-hover",
        danger ? "text-err" : "text-fg",
      )}
    >
      <span className={danger ? "text-err" : "text-muted"}>{icon}</span>
      {children}
    </button>
  );
}

function MenuLink({ children, icon, href, external }: { children: React.ReactNode; icon: React.ReactNode; href: string; external?: boolean }) {
  const cls = "flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] text-fg transition-colors hover:bg-hover";
  return external ? (
    <a href={href} target="_blank" rel="noreferrer" className={cls}>
      <span className="text-muted">{icon}</span>
      {children}
    </a>
  ) : (
    <Link href={href} className={cls}>
      <span className="text-muted">{icon}</span>
      {children}
    </Link>
  );
}


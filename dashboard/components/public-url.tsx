"use client";

import { useState } from "react";
import { AlertTriangle, ExternalLink, Globe2, Loader2, Radio } from "lucide-react";
import { api, type Project } from "@/lib/api";
import { hostOf } from "@/lib/format";
import { Button, Card } from "./ui";
import { CopyButton, useFeedback } from "./feedback";

/**
 * Free public HTTPS URL for a project (Cloudflare quick tunnel). Anyone on the
 * internet can open it while this computer is running Mini Vercel.
 */
export function PublicUrlCard({ project, onChange }: { project: Project; onChange: () => void }) {
  const { toast, confirm } = useFeedback();
  const [busy, setBusy] = useState(false);
  const live = project.publicEnabled && project.publicStatus === "live" && project.publicUrl;
  const starting = project.publicEnabled && (project.publicStatus === "starting" || (!project.publicUrl && project.publicStatus !== "error"));
  const failed = project.publicEnabled && project.publicStatus === "error";

  const toggle = async (enabled: boolean) => {
    if (!enabled) {
      const ok = await confirm({
        title: "Stop public sharing?",
        description: "The public link will stop working immediately. If you turn sharing on again you'll get a new link.",
        confirmLabel: "Stop sharing",
        danger: true,
      });
      if (!ok) return;
    }
    setBusy(true);
    try {
      await api.setPublic(project.id, enabled);
      if (enabled) toast("info", "Creating your public link…", "This usually takes 5–15 seconds.");
      onChange();
    } catch (e) {
      toast("error", "Could not change sharing", e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="overflow-hidden">
      <div className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center">
        <div
          className={
            "flex size-10 shrink-0 items-center justify-center rounded-full border " +
            (live ? "border-ok/30 bg-ok-bg text-ok" : "border-line bg-subtle text-muted")
          }
        >
          {starting ? <Loader2 className="size-5 animate-spin" /> : live ? <Radio className="size-5" /> : <Globe2 className="size-5" />}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-[15px] font-semibold">
            Temporary share link
            {live && <span className="rounded-full bg-ok-bg px-2 py-0.5 text-[11px] font-medium text-ok">Live on the internet</span>}
          </div>
          {live ? (
            <div className="mt-1 flex min-w-0 items-center gap-1">
              <a href={project.publicUrl} target="_blank" rel="noreferrer" className="truncate font-mono text-[13px] font-medium text-link hover:underline">
                {hostOf(project.publicUrl)}
              </a>
              <CopyButton value={project.publicUrl} />
            </div>
          ) : starting ? (
            <p className="mt-1 text-[13px] text-muted">Creating a secure tunnel through Cloudflare…</p>
          ) : failed ? (
            <p className="mt-1 flex items-center gap-1.5 text-[13px] text-err">
              <AlertTriangle className="size-3.5" /> Could not create the tunnel. It will retry automatically. Check your internet connection.
            </p>
          ) : (
            <p className="mt-1 text-[13px] text-muted">
              Get a free <span className="font-medium text-fg">https://</span> link anyone can open, from any device. No domain or account needed.
            </p>
          )}
        </div>

        <div className="flex shrink-0 gap-2">
          {live && (
            <a
              href={project.publicUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex h-8 items-center gap-1.5 rounded-md border border-line px-3 text-[13px] font-medium hover:bg-hover"
            >
              <ExternalLink className="size-3.5" /> Open
            </a>
          )}
          {project.publicEnabled ? (
            <Button size="sm" variant="ghost" onClick={() => toggle(false)} loading={busy}>
              Stop sharing
            </Button>
          ) : (
            <Button size="sm" variant="primary" icon={<Globe2 className="size-4" />} onClick={() => toggle(true)} loading={busy} disabled={!project.productionDeployment}>
              Go Public
            </Button>
          )}
        </div>
      </div>
      <div className="border-t border-line bg-subtle px-5 py-2.5 text-xs text-muted">
        {project.productionDeployment
          ? "The link always serves your current production deployment, so rollbacks apply instantly. It works while this computer is running Mini Vercel."
          : "Deploy the project first. The public link will serve its production deployment."}
      </div>
    </Card>
  );
}

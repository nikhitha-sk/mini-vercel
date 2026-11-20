"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { AlertTriangle, CheckCircle2, ExternalLink, Globe, KeyRound, Loader2, Rocket, Unplug } from "lucide-react";
import { api, type NetlifyStatus, type Project } from "@/lib/api";
import { hostOf } from "@/lib/format";
import { Button, ButtonLink, Card, CardHeader, Input, Label } from "./ui";
import { CopyButton, useFeedback } from "./feedback";

/** Permanent live URL for a project (published to Netlify after each build). */
export function LiveUrlCard({ project, onChange }: { project: Project; onChange: () => void }) {
  const { toast } = useFeedback();
  const [netlify, setNetlify] = useState<NetlifyStatus | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api.netlify().then(setNetlify).catch(() => setNetlify({ connected: false }));
  }, []);

  const publishing = project.liveStatus === "publishing";
  const live = !!project.liveUrl && project.liveStatus !== "publishing";
  const failed = project.liveStatus === "error";

  const publishNow = async () => {
    setBusy(true);
    try {
      await api.publishLive(project.id);
      toast("info", "Publishing to the internet…", "Usually takes 10–30 seconds.");
      onChange();
    } catch (e) {
      toast("error", "Could not publish", e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="overflow-hidden border-ok/30">
      <div className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center">
        <div className={"flex size-11 shrink-0 items-center justify-center rounded-full " + (live ? "bg-ok text-white" : "border border-line bg-subtle text-muted")}>
          {publishing ? <Loader2 className="size-5 animate-spin" /> : <Globe className="size-5" />}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-[15px] font-semibold">
            Live URL
            {live && <span className="rounded-full bg-ok-bg px-2 py-0.5 text-[11px] font-medium text-ok">Permanent · works everywhere</span>}
          </div>
          {live ? (
            <div className="mt-1 flex min-w-0 items-center gap-1">
              <a href={project.liveUrl} target="_blank" rel="noreferrer" className="truncate font-mono text-sm font-medium text-link hover:underline">
                {hostOf(project.liveUrl)}
              </a>
              <CopyButton value={project.liveUrl} />
            </div>
          ) : publishing ? (
            <p className="mt-1 text-[13px] text-muted">Publishing your site to the internet…</p>
          ) : netlify && !netlify.connected ? (
            <p className="mt-1 text-[13px] text-muted">Connect a free Netlify account once, and every project gets a permanent https link that works on any device, even when this PC is off.</p>
          ) : (
            <p className="mt-1 text-[13px] text-muted">Publish this project to get a permanent https link.</p>
          )}
          {failed && (
            <p className="mt-1 flex items-center gap-1.5 text-[13px] text-err">
              <AlertTriangle className="size-3.5" /> The last publish failed. See the build logs, then try again.
            </p>
          )}
        </div>
        <div className="flex shrink-0 gap-2">
          {live && (
            <ButtonLink href={project.liveUrl} external variant="primary" size="sm" icon={<ExternalLink className="size-4" />}>
              Open
            </ButtonLink>
          )}
          {netlify && !netlify.connected && (
            <ButtonLink href="/platform#netlify" variant="primary" size="sm" icon={<KeyRound className="size-4" />}>
              Connect Netlify
            </ButtonLink>
          )}
          {netlify?.connected && !publishing && (
            <Button size="sm" variant={live ? "secondary" : "primary"} icon={<Rocket className="size-4" />} onClick={publishNow} loading={busy} disabled={!project.productionDeployment}>
              {live ? "Republish" : "Publish now"}
            </Button>
          )}
        </div>
      </div>
    </Card>
  );
}

/** Platform-level Netlify connection (one free token for all projects). */
export function NetlifyConnectCard() {
  const { toast, confirm } = useFeedback();
  const [status, setStatus] = useState<NetlifyStatus | null>(null);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const load = () => api.netlify().then(setStatus).catch(() => setStatus({ connected: false }));
  useEffect(() => {
    load();
  }, []);

  const connect = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim()) return;
    setBusy(true);
    try {
      const s = await api.connectNetlify(token.trim());
      setStatus(s);
      setToken("");
      toast("success", "Netlify connected", "New deployments now get permanent live URLs.");
    } catch (err) {
      toast("error", "Could not connect", err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async () => {
    if (!(await confirm({ title: "Disconnect Netlify?", description: "Existing live sites keep working, but new deployments won't be published.", confirmLabel: "Disconnect", danger: true }))) return;
    await api.disconnectNetlify();
    load();
  };

  return (
    <div id="netlify">
      <Card>
        <CardHeader
          title={
            <span className="inline-flex items-center gap-2">
              <Globe className="size-4" /> Permanent live URLs (Netlify, free)
            </span>
          }
          description="Mini Vercel builds your sites, then publishes them to Netlify's free global CDN at https://<project>.netlify.app. The links work on any device, even when this PC is off."
          actions={
            status?.connected && (
              <Button variant="ghost" size="sm" icon={<Unplug className="size-4" />} onClick={disconnect}>
                Disconnect
              </Button>
            )
          }
        />
        {status?.connected ? (
          <div className="flex items-center gap-3 p-5 text-sm">
            <CheckCircle2 className="size-5 text-ok" />
            <div>
              <div className="font-medium">Connected{status.name ? ` as ${status.name}` : ""}</div>
              <div className="text-[13px] text-muted">{status.email || "Every new deployment is published automatically."}</div>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-6 p-5 md:grid-cols-2">
            <ol className="space-y-3 text-[13px] text-muted">
              <li>
                <span className="font-medium text-fg">1.</span> Create a free account at{" "}
                <a href="https://app.netlify.com/signup" target="_blank" rel="noreferrer" className="text-link hover:underline">
                  netlify.com
                </a>{" "}
                (sign up with GitHub, no credit card).
              </li>
              <li>
                <span className="font-medium text-fg">2.</span> Open{" "}
                <a href="https://app.netlify.com/user/applications#personal-access-tokens" target="_blank" rel="noreferrer" className="text-link hover:underline">
                  User settings → Applications → Personal access tokens
                </a>
                .
              </li>
              <li>
                <span className="font-medium text-fg">3.</span> Click <span className="font-medium text-fg">New access token</span>, name it “Mini Vercel”, set expiration to “No expiration”, and copy it.
              </li>
              <li>
                <span className="font-medium text-fg">4.</span> Paste it here. It is stored encrypted (AES-256-GCM).
              </li>
            </ol>
            <form onSubmit={connect} className="space-y-3">
              <div>
                <Label htmlFor="nf-token">Netlify personal access token</Label>
                <Input id="nf-token" type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder="nfp_…" mono autoComplete="off" />
              </div>
              <Button type="submit" variant="primary" loading={busy} icon={<KeyRound className="size-4" />} className="w-full">
                Connect Netlify
              </Button>
              <p className="text-xs text-muted">
                After connecting, open a project and click <span className="font-medium text-fg">Publish now</span>, or just deploy. Publishing is automatic.{" "}
                <Link href="/" className="underline underline-offset-2">
                  Back to projects
                </Link>
              </p>
            </form>
          </div>
        )}
      </Card>
    </div>
  );
}

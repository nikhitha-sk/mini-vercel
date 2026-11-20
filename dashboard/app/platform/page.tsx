"use client";

import { ArrowRight, Boxes, Cpu, Database, Globe, HardDrive, Layers, Server, ShieldCheck, Timer, Workflow } from "lucide-react";
import { AppShell, Container, PageHeader } from "@/components/shell";
import { Card, CardHeader, Kv, Skeleton } from "@/components/ui";
import { api } from "@/lib/api";
import { useResource } from "@/lib/hooks";
import { NetlifyConnectCard } from "@/components/live-hosting";

const FLOW = [
  { icon: <Layers className="size-4" />, title: "Dashboard / API", body: "Records an immutable deployment row (QUEUED) and enqueues one job." },
  { icon: <Workflow className="size-4" />, title: "Queue", body: "Redis + asynq. Retries infra errors with exponential backoff." },
  { icon: <Boxes className="size-4" />, title: "Build worker", body: "Clones the repo, starts a fresh locked-down container, streams logs." },
  { icon: <HardDrive className="size-4" />, title: "Object storage", body: "Output uploaded under /<deploymentId>/. Never overwritten." },
  { icon: <Globe className="size-4" />, title: "Edge router", body: "Host → alias → deployment, LRU-cached, invalidated on promote." },
];

export default function PlatformPage() {
  const cfg = useResource(() => api.config(), []);
  const c = cfg.data;
  return (
    <AppShell>
      <PageHeader title="Platform" description="How Mini Vercel builds and serves your sites, and the limits every build runs under." />
      <Container className="space-y-6">
        <NetlifyConnectCard />

        <Card>
          <CardHeader title="Deployment pipeline" description="The API never builds anything. Workers do, and the edge only reads, so each part scales on its own." />
          <ol className="grid grid-cols-1 gap-px overflow-hidden bg-line md:grid-cols-5">
            {FLOW.map((s, i) => (
              <li key={s.title} className="relative bg-elevated p-5">
                <div className="flex items-center gap-2 text-[13px] font-semibold">
                  <span className="flex size-7 items-center justify-center rounded-md border border-line bg-subtle text-muted">{s.icon}</span>
                  {s.title}
                </div>
                <p className="mt-2 text-[13px] leading-relaxed text-muted">{s.body}</p>
                {i < FLOW.length - 1 && <ArrowRight className="absolute -right-2 top-6 z-10 hidden size-4 rounded-full bg-elevated text-faint md:block" />}
              </li>
            ))}
          </ol>
        </Card>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Card>
            <CardHeader
              title={
                <span className="inline-flex items-center gap-2">
                  <ShieldCheck className="size-4 text-ok" /> Build sandbox
                </span>
              }
              description="Every build runs in a new container that is destroyed afterwards, even on failure."
            />
            <div className="grid grid-cols-2 gap-5 p-5">
              {c ? (
                <>
                  <Kv label="Image" mono>{c.limits.image}</Kv>
                  <Kv label="User" mono>node (non-root)</Kv>
                  <Kv label="CPU">
                    <span className="inline-flex items-center gap-1.5"><Cpu className="size-3.5 text-muted" />{c.limits.cpus} vCPU</span>
                  </Kv>
                  <Kv label="Memory">{c.limits.memoryMB} MB, no swap</Kv>
                  <Kv label="Processes">{c.limits.pids} max</Kv>
                  <Kv label="Time limit">
                    <span className="inline-flex items-center gap-1.5"><Timer className="size-3.5 text-muted" />{Math.round(c.limits.timeoutSec / 60)} minutes</span>
                  </Kv>
                  <Kv label="Capabilities">All dropped</Kv>
                  <Kv label="Docker socket">Never mounted</Kv>
                </>
              ) : (
                [0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-10" />)
              )}
            </div>
          </Card>

          <Card>
            <CardHeader
              title={
                <span className="inline-flex items-center gap-2">
                  <Server className="size-4" /> Routing
                </span>
              }
              description="Preview URLs are permanent. The production domain is an alias, so rollback is one database update."
            />
            <div className="space-y-4 p-5 text-[13px]">
              <RouteRow host={c ? `dpl-xxxxxxxxxx.${c.rootDomain}` : "…"} target="One exact, immutable deployment" />
              <RouteRow host={c ? `<project>.${c.rootDomain}` : "…"} target="Alias → current production deployment" />
              <div className="rounded-lg border border-line bg-subtle p-3 text-muted">
                <span className="inline-flex items-center gap-1.5 font-medium text-fg">
                  <Database className="size-3.5" /> aliases
                </span>{" "}
                (hostname → deployment_id). Promote and rollback are a single <span className="font-mono">UPSERT</span>; edge caches are invalidated over Redis
                pub/sub.
              </div>
            </div>
          </Card>
        </div>

        <Card>
          <CardHeader title="Scope" description="Static hosting platform. Intentionally out of scope: serverless functions and server-side rendering." />
          <div className="grid grid-cols-1 gap-4 p-5 text-[13px] text-muted sm:grid-cols-3">
            <p><span className="font-medium text-fg">Supported:</span> Vite, Astro, Next.js static export, Create React App, Docusaurus, plain HTML.</p>
            <p><span className="font-medium text-fg">SPA routing:</span> unknown paths fall back to <span className="font-mono">index.html</span>; a shipped <span className="font-mono">404.html</span> is honoured for assets.</p>
            <p><span className="font-medium text-fg">Caching:</span> hashed assets get <span className="font-mono">immutable</span>, HTML is revalidated with ETags.</p>
          </div>
        </Card>
      </Container>
    </AppShell>
  );
}

function RouteRow({ host, target }: { host: string; target: string }) {
  return (
    <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
      <span className="rounded-md border border-line bg-subtle px-2 py-1 font-mono text-xs">{host}</span>
      <ArrowRight className="hidden size-3.5 text-faint sm:block" />
      <span className="text-muted">{target}</span>
    </div>
  );
}

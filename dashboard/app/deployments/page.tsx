"use client";

import { useMemo, useState } from "react";
import { Rocket } from "lucide-react";
import { AppShell, Container, PageHeader } from "@/components/shell";
import { ButtonLink, Card, EmptyState, ErrorBanner, Skeleton } from "@/components/ui";
import { DeploymentList } from "@/components/deployment-list";
import { api, isActive, type Status } from "@/lib/api";
import { useResource, useTick } from "@/lib/hooks";
import { cn } from "@/lib/format";

const FILTERS: { label: string; match: (s: Status) => boolean }[] = [
  { label: "All", match: () => true },
  { label: "Ready", match: (s) => s === "READY" },
  { label: "In progress", match: (s) => isActive(s) },
  { label: "Error", match: (s) => s === "FAILED" },
  { label: "Canceled", match: (s) => s === "CANCELED" },
];

export default function DeploymentsPage() {
  useTick();
  const [filter, setFilter] = useState(0);
  const recent = useResource(() => api.recent(), [], { pollMs: 2500, pollWhile: (ds) => ds.some((d) => isActive(d.status)) });
  const list = useMemo(() => (recent.data ?? []).filter((d) => FILTERS[filter].match(d.status)), [recent.data, filter]);

  return (
    <AppShell>
      <PageHeader title="Deployments" description="Recent deployments across all of your projects." />
      <Container>
        <div className="mb-4 flex flex-wrap gap-1">
          {FILTERS.map((f, i) => (
            <button
              key={f.label}
              onClick={() => setFilter(i)}
              className={cn(
                "h-8 cursor-pointer rounded-full border px-3 text-[13px] transition-colors",
                filter === i ? "border-fg bg-fg text-bg" : "border-line text-muted hover:border-line-strong hover:text-fg",
              )}
            >
              {f.label}
            </button>
          ))}
        </div>
        {recent.error && <ErrorBanner message={recent.error} onRetry={recent.reload} />}
        <Card className="overflow-hidden">
          {!recent.data && !recent.error && <div className="space-y-3 p-5">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-12" />)}</div>}
          {recent.data && list.length === 0 && (
            <EmptyState
              icon={<Rocket className="size-5" />}
              title={recent.data.length === 0 ? "No deployments yet" : "Nothing here"}
              description={recent.data.length === 0 ? "Create a project to trigger your first build." : "No deployments match this filter."}
              action={recent.data.length === 0 && <ButtonLink href="/new" variant="primary">New Project</ButtonLink>}
            />
          )}
          {list.length > 0 && <DeploymentList deployments={list} onChanged={recent.reload} showProject />}
        </Card>
      </Container>
    </AppShell>
  );
}

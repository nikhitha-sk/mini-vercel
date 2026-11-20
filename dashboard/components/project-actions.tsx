"use client";

import { useCallback } from "react";
import { useFeedback } from "./feedback";
import { api, type Project } from "@/lib/api";
import { hostOf } from "@/lib/format";

/**
 * Returns a function that asks for confirmation (the user types the project
 * name, or "delete N projects" for a bulk delete) and then deletes the given
 * projects. It resolves with the ids that were actually deleted.
 */
export function useDeleteProjects() {
  const { confirm, toast } = useFeedback();
  return useCallback(
    async (ps: Project[]): Promise<string[]> => {
      if (ps.length === 0) return [];
      const one = ps.length === 1;
      const ok = await confirm({
        title: one ? `Delete ${ps[0].name}?` : `Delete ${ps.length} projects?`,
        description: <DeleteSummary ps={ps} />,
        confirmLabel: one ? "Delete Project" : `Delete ${ps.length} Projects`,
        danger: true,
        requireText: one ? ps[0].name : `delete ${ps.length} projects`,
      });
      if (!ok) return [];

      const results = await Promise.allSettled(ps.map((p) => api.deleteProject(p.id)));
      const deleted = ps.filter((_, i) => results[i].status === "fulfilled").map((p) => p.id);
      const failed = results.filter((r): r is PromiseRejectedResult => r.status === "rejected");
      if (deleted.length > 0) {
        toast("success", one ? `Deleted ${ps[0].name}` : `Deleted ${deleted.length} project${deleted.length === 1 ? "" : "s"}`);
      }
      if (failed.length > 0) {
        const reason = failed[0].reason;
        toast("error", `Could not delete ${failed.length} project${failed.length === 1 ? "" : "s"}`, reason instanceof Error ? reason.message : String(reason));
      }
      return deleted;
    },
    [confirm, toast],
  );
}

function DeleteSummary({ ps }: { ps: Project[] }) {
  const one = ps.length === 1;
  const live = ps.filter((p) => p.liveUrl);
  return (
    <div className="space-y-3">
      {one ? (
        <p>
          This permanently removes <span className="font-medium text-fg">{ps[0].name}</span> and everything in it:
        </p>
      ) : (
        <>
          <p>These projects and everything in them will be permanently removed:</p>
          <ul className="max-h-36 space-y-1 overflow-y-auto rounded-md border border-line bg-subtle px-3 py-2 text-[13px]">
            {ps.map((p) => (
              <li key={p.id} className="truncate font-medium text-fg">
                {p.name}
              </li>
            ))}
          </ul>
        </>
      )}
      <ul className="list-disc space-y-1 pl-5 text-[13px]">
        <li>All deployments, build logs and stored build files</li>
        <li>Environment variables</li>
        <li>{one ? <>The project URL <span className="font-mono text-fg">{hostOf(ps[0].productionUrl)}</span></> : "Their project URLs and public links"}</li>
        {live.length > 0 && (
          <li>
            {one ? "The permanent Netlify site" : `${live.length} permanent Netlify site${live.length === 1 ? "" : "s"}`}
            {one && live[0].liveUrl && <> (<span className="font-mono text-fg">{hostOf(live[0].liveUrl)}</span>)</>}
          </li>
        )}
      </ul>
      <p className="font-medium text-err">This cannot be undone.</p>
    </div>
  );
}

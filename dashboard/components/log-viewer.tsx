"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowDownToLine, Clock, Download, Search, TerminalSquare } from "lucide-react";
import { api, type Deployment, type LogLine } from "@/lib/api";
import { clockTime, cn } from "@/lib/format";

/**
 * Live build log viewer. Replays stored lines and follows new ones over
 * Server-Sent Events until the deployment reaches a terminal state.
 */
export function LogViewer({
  deploymentId,
  onStatus,
  live,
}: {
  deploymentId: string;
  onStatus?: (d: Deployment) => void;
  live: boolean;
}) {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [connected, setConnected] = useState(false);
  const [follow, setFollow] = useState(true);
  const [showTime, setShowTime] = useState(true);
  const [query, setQuery] = useState("");
  const boxRef = useRef<HTMLDivElement>(null);
  const onStatusRef = useRef(onStatus);
  onStatusRef.current = onStatus;

  useEffect(() => {
    setLines([]);
    const seen = new Set<number>();
    const es = new EventSource(api.eventsUrl(deploymentId));
    let buf: LogLine[] = [];
    let raf = 0;
    const flush = () => {
      raf = 0;
      if (!buf.length) return;
      const add = buf;
      buf = [];
      setLines((prev) => [...prev, ...add]);
    };
    es.onopen = () => setConnected(true);
    es.addEventListener("log", (e) => {
      const l = JSON.parse((e as MessageEvent).data) as LogLine;
      if (seen.has(l.seq)) return;
      seen.add(l.seq);
      buf.push(l);
      if (!raf) raf = requestAnimationFrame(flush);
    });
    es.addEventListener("status", (e) => onStatusRef.current?.(JSON.parse((e as MessageEvent).data)));
    es.addEventListener("end", () => {
      flush();
      setConnected(false);
      es.close();
    });
    es.onerror = () => setConnected(false);
    return () => {
      cancelAnimationFrame(raf);
      es.close();
    };
  }, [deploymentId]);

  useEffect(() => {
    if (follow && boxRef.current) boxRef.current.scrollTop = boxRef.current.scrollHeight;
  }, [lines, follow]);

  const filtered = useMemo(() => {
    if (!query) return lines;
    const q = query.toLowerCase();
    return lines.filter((l) => l.line.toLowerCase().includes(q));
  }, [lines, query]);

  const errors = lines.filter((l) => l.stream === "stderr").length;

  const download = () => {
    const text = lines.map((l) => `${l.ts} [${l.stream}] ${l.line}`).join("\n");
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = `${deploymentId}.log`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="overflow-hidden rounded-xl border border-line bg-term shadow-card">
      <div className="flex flex-wrap items-center gap-2 border-b border-white/10 px-3 py-2 text-[12px] text-zinc-400">
        <TerminalSquare className="size-4 text-zinc-500" />
        <span className="font-medium text-zinc-200">Build Logs</span>
        <span className="text-zinc-600">·</span>
        <span>{lines.length} lines</span>
        {errors > 0 && <span className="text-red-400">{errors} stderr</span>}
        {live && (
          <span className="flex items-center gap-1.5">
            <span className={cn("size-1.5 rounded-full", connected ? "animate-pulse-dot bg-green-400" : "bg-zinc-600")} />
            {connected ? "Live" : "Connecting…"}
          </span>
        )}
        <div className="ml-auto flex items-center gap-1">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-zinc-500" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Find in logs"
              className="h-7 w-36 rounded-md border border-white/10 bg-white/5 pl-7 pr-2 text-[12px] text-zinc-200 placeholder:text-zinc-500 focus:border-white/25 focus:outline-none sm:w-48"
            />
          </div>
          <ToolButton active={showTime} onClick={() => setShowTime((v) => !v)} label="Toggle timestamps">
            <Clock className="size-3.5" />
          </ToolButton>
          <ToolButton active={follow} onClick={() => setFollow((v) => !v)} label="Follow output">
            <ArrowDownToLine className="size-3.5" />
          </ToolButton>
          <ToolButton onClick={download} label="Download logs">
            <Download className="size-3.5" />
          </ToolButton>
        </div>
      </div>
      <div
        ref={boxRef}
        onWheel={(e) => {
          if (e.deltaY < 0) setFollow(false);
        }}
        className="scrollbar-thin h-[460px] overflow-auto py-2 font-mono text-[12.5px] leading-[1.7] text-term-fg"
      >
        {filtered.length === 0 && (
          <div className="px-4 py-3 text-zinc-500">{lines.length === 0 ? (live ? "Waiting for build output…" : "No logs recorded.") : "No matching lines."}</div>
        )}
        {filtered.map((l) => (
          <div key={l.seq} className={cn("group flex gap-3 px-3 hover:bg-white/[0.04]", l.stream === "stderr" && "bg-red-500/[0.06]")}>
            <span className="w-8 shrink-0 select-none text-right text-zinc-600">{l.seq}</span>
            {showTime && <span className="shrink-0 select-none text-zinc-500">{clockTime(l.ts)}</span>}
            <span
              className={cn(
                "min-w-0 whitespace-pre-wrap break-all",
                l.stream === "stderr" && "text-red-400",
                l.stream === "system" && "text-sky-300",
              )}
            >
              {l.line || " "}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

function ToolButton({ children, onClick, active, label }: { children: React.ReactNode; onClick: () => void; active?: boolean; label: string }) {
  return (
    <button
      onClick={onClick}
      title={label}
      aria-label={label}
      className={cn(
        "flex size-7 cursor-pointer items-center justify-center rounded-md transition-colors hover:bg-white/10 hover:text-zinc-100",
        active ? "text-zinc-100" : "text-zinc-500",
      )}
    >
      {children}
    </button>
  );
}

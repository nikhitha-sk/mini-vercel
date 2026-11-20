"use client";

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Small data-fetching hook with optional polling. Polling only runs while
 * `pollWhile(data)` is true, so idle pages make no background requests.
 */
export function useResource<T>(
  fetcher: (() => Promise<T>) | null,
  deps: unknown[],
  opts: { pollMs?: number; pollWhile?: (d: T) => boolean } = {},
) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;
  const optsRef = useRef(opts);
  optsRef.current = opts;

  const load = useCallback(async () => {
    if (!fetcherRef.current) return;
    try {
      const d = await fetcherRef.current();
      setData(d);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setLoading(true);
    setData(null);
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  useEffect(() => {
    const { pollMs, pollWhile } = optsRef.current;
    if (!pollMs || !data || (pollWhile && !pollWhile(data))) return;
    const t = setTimeout(load, pollMs);
    return () => clearTimeout(t);
  }, [data, load]);

  return { data, error, loading, reload: load, setData };
}

/** Re-render every `ms` so relative timestamps stay fresh. */
export function useTick(ms = 30000) {
  const [, set] = useState(0);
  useEffect(() => {
    const t = setInterval(() => set((n) => n + 1), ms);
    return () => clearInterval(t);
  }, [ms]);
}

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * The state of one asynchronously-loaded resource: its data (null until loaded),
 * whether a load is in flight, whether the last load failed, and a manual reload
 * (retry / pull-to-refresh).
 */
export interface AsyncResource<T> {
  data: T | null;
  loading: boolean;
  error: boolean;
  reload: () => Promise<void>;
}

/**
 * Loads a resource whose fetcher may be absent. Passing `null` for `load` (e.g.
 * no child selected, or signed out) is a first-class state: the hook settles to
 * empty/non-loading/non-error and fires no request — the same auth-gated pattern
 * the notifications slice uses. When `load` becomes a function, the resource
 * (re)loads.
 *
 * Callers MUST memoize `load` (useMemo/useCallback keyed on its inputs) so the
 * effect re-runs only when the inputs actually change.
 */
export function useAsyncResource<T>(load: (() => Promise<T>) | null): AsyncResource<T> {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState<boolean>(load !== null);
  const [error, setError] = useState(false);

  // Monotonic generation shared by the effect and the manual reload. Every new
  // load (a childId switch, a logout, a retry) and every effect cleanup advances
  // it, so a superseded in-flight call — captured against an older generation —
  // can detect it is stale and skip its state writes. Without this a slow prior
  // response could overwrite fresher data (bug_001).
  const generationRef = useRef(0);

  const reload = useCallback(async () => {
    const generation = (generationRef.current += 1);
    const isCurrent = (): boolean => generation === generationRef.current;
    if (load === null) {
      setData(null);
      setLoading(false);
      setError(false);
      return;
    }
    setLoading(true);
    setError(false);
    try {
      const result = await load();
      if (isCurrent()) {
        setData(result);
      }
    } catch {
      if (isCurrent()) {
        setError(true);
      }
    } finally {
      if (isCurrent()) {
        setLoading(false);
      }
    }
  }, [load]);

  useEffect(() => {
    void reload();
    // Cancel the in-flight load on cleanup (load changed / unmount) so its late
    // response is discarded rather than applied over the next load's state.
    return () => {
      generationRef.current += 1;
    };
  }, [reload]);

  return { data, loading, error, reload };
}

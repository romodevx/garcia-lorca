import { useCallback, useEffect, useState } from "react";

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

  const reload = useCallback(async () => {
    if (load === null) {
      setData(null);
      setLoading(false);
      setError(false);
      return;
    }
    setLoading(true);
    setError(false);
    try {
      setData(await load());
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, [load]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { data, loading, error, reload };
}

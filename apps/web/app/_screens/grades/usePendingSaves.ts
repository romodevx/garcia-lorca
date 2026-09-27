import { useCallback, useEffect, useState } from "react";

export interface PendingSaves {
  /** Saves still in flight. */
  pending: number;
  /** Counts `work` as in flight until it settles; returns it unchanged. */
  track: <T>(work: Promise<T>) => Promise<T>;
}

/**
 * The saves still travelling, and the browser's "leave page?" prompt while any
 * are. Lives above the month's table so a month switch never forgets a save
 * that has not landed yet.
 */
export function usePendingSaves(): PendingSaves {
  const [pending, setPending] = useState(0);

  const track = useCallback(<T,>(work: Promise<T>): Promise<T> => {
    setPending((n) => n + 1);
    return work.finally(() => setPending((n) => n - 1));
  }, []);

  useEffect(() => {
    if (pending === 0) return;
    const guard = (event: BeforeUnloadEvent): void => {
      event.preventDefault();
      // Older browsers only prompt when returnValue is set.
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [pending]);

  return { pending, track };
}

import { useCallback, useRef, useState } from "react";

import { isRejectedValue, type GradeSheet, type GradeSheetClient } from "../../grades/monthly";

export type CellStatus =
  | { kind: "idle" }
  | { kind: "saving" }
  | { kind: "saved" }
  /** Network or server trouble: the same value can be sent again. */
  | { kind: "failed" }
  /** The server refused the value itself; its words go on the row. */
  | { kind: "rejected"; message: string };

type Grade = number | null;

const IDLE: CellStatus = { kind: "idle" };

export interface CellSaves {
  /** The grades the server has confirmed, by enrollment — what the counters count. */
  values: Readonly<Record<string, Grade>>;
  statusOf: (enrollmentId: string) => CellStatus;
  /** Saves (or, for null, clears) a cell — a no-op when nothing changed. */
  commit: (enrollmentId: string, next: Grade) => void;
  retry: (enrollmentId: string) => void;
  /** The teacher is typing again: the server's old complaint no longer applies. */
  dismissRejection: (enrollmentId: string) => void;
}

/**
 * Autosave per cell. Each enrollment remembers the last value it ASKED for, so
 * Enter followed by the blur it causes sends one request, not two; and each
 * request is numbered per cell so a slow earlier save can never overwrite the
 * status of a later one.
 */
export function useCellSaves(client: GradeSheetClient, sheet: GradeSheet, track: <T>(work: Promise<T>) => Promise<T>): CellSaves {
  const [values, setValues] = useState<Record<string, Grade>>(() => Object.fromEntries(sheet.rows.map((r) => [r.enrollmentId, r.value])));
  const [status, setStatus] = useState<Record<string, CellStatus>>({});
  // Mirrors read by the callbacks, so they stay stable across renders.
  const confirmed = useRef(new Map<string, Grade>(sheet.rows.map((r) => [r.enrollmentId, r.value])));
  const requested = useRef(new Map<string, Grade>(confirmed.current));
  const statusById = useRef(new Map<string, CellStatus>());
  const sequence = useRef(new Map<string, number>());

  const setCell = useCallback((id: string, next: CellStatus) => {
    statusById.current.set(id, next);
    setStatus((prev) => ({ ...prev, [id]: next }));
  }, []);

  const send = useCallback((id: string, next: Grade) => {
    const request = (sequence.current.get(id) ?? 0) + 1;
    sequence.current.set(id, request);
    requested.current.set(id, next);
    setCell(id, { kind: "saving" });
    const key = { assignmentId: sheet.assignmentId, month: sheet.month, enrollmentId: id };
    const work: Promise<unknown> = next === null ? client.remove(key) : client.save({ ...key, value: next });
    void track(work).then(
      () => {
        if (sequence.current.get(id) !== request) return;
        confirmed.current.set(id, next);
        setValues((prev) => ({ ...prev, [id]: next }));
        setCell(id, { kind: "saved" });
      },
      (err: unknown) => {
        if (sequence.current.get(id) !== request) return;
        setCell(id, isRejectedValue(err) ? { kind: "rejected", message: err.detail } : { kind: "failed" });
      },
    );
  }, [client, sheet.assignmentId, sheet.month, setCell, track]);

  const commit = useCallback((id: string, next: Grade) => {
    const current = statusById.current.get(id)?.kind;
    const unsettled = current === "failed" || current === "rejected";
    if (requested.current.get(id) === next && !unsettled) return;
    send(id, next);
  }, [send]);

  const retry = useCallback((id: string) => send(id, requested.current.get(id) ?? null), [send]);

  const dismissRejection = useCallback((id: string) => {
    if (statusById.current.get(id)?.kind !== "rejected") return;
    // What the server refused was never stored: compare the next commit with what was.
    requested.current.set(id, confirmed.current.get(id) ?? null);
    setCell(id, IDLE);
  }, [setCell]);

  const statusOf = useCallback((id: string): CellStatus => status[id] ?? IDLE, [status]);

  return { values, statusOf, commit, retry, dismissRejection };
}

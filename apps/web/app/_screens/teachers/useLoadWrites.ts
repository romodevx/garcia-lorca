"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { API_URL } from "../../api-url";
import {
  assignTeachingLoad,
  deleteTeachingAssignment,
  type TeachingLoadConflict,
  type TeachingLoadItem,
} from "../../teachers/api";
import type { LoadCatalogue, LoadSlot } from "../../teachers/load";
import { describeLoadWriteError, isAlreadyGone, isConcurrentChange } from "../../teachers/loadErrors";
import type { NamedConflict } from "./ConflictNotice";

/** The undo window for a freed class — at least eight seconds (ui-forms rules). */
const UNDO_MS = 8000;
const TOAST_MS = 2600;

export interface LoadWrites {
  /** Sends slots for the teacher; throws when the request itself failed. */
  assign: (items: TeachingLoadItem[]) => Promise<void>;
  remove: (slot: LoadSlot) => void;
  undo: () => Promise<void>;
  replaceConflicts: () => Promise<void>;
  keepConflicts: () => void;
  undoable: LoadSlot | undefined;
  /** Assignment ids whose DELETE is in flight; their × ignores clicks. */
  removing: ReadonlySet<string>;
  conflicts: readonly NamedConflict[];
  toast: string | undefined;
  error: string | undefined;
}

export interface LoadWritesDeps {
  teacherId: string;
  /** The cycle the page shows; pending conflicts and undo belong to it alone. */
  cycleId: string;
  catalogue: LoadCatalogue;
  reload: () => Promise<void>;
  drop: (assignmentId: string) => void;
}

function assignedMessage(count: number): string {
  return count === 1 ? "1 grupo asignado" : `${count} grupos asignados`;
}

/**
 * The page's writes. A freed class is deleted AT ONCE (closing the tab must not
 * leave it assigned) and Deshacer gives it back through the bulk call; a slot
 * someone else holds comes back as a conflict the person resolves explicitly.
 */
export function useLoadWrites({ teacherId, cycleId, catalogue, reload, drop }: LoadWritesDeps): LoadWrites {
  const [raw, setRaw] = useState<TeachingLoadConflict[]>([]);
  const [undoable, setUndoable] = useState<LoadSlot | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const undoTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const shownCycle = useRef(cycleId);
  // The ref guards synchronously (two clicks can land before a re-render);
  // the state is what the chips render from.
  const inFlight = useRef(new Set<string>());
  const [removing, setRemoving] = useState<ReadonlySet<string>>(new Set());

  useEffect(() => () => {
    clearTimeout(undoTimer.current);
    clearTimeout(toastTimer.current);
  }, []);

  // A conflict or an undo of another cycle must never act on this one.
  useEffect(() => {
    shownCycle.current = cycleId;
    clearTimeout(undoTimer.current);
    setRaw([]);
    setUndoable(undefined);
    setError(undefined);
  }, [cycleId]);

  const say = useCallback((message: string): void => {
    clearTimeout(toastTimer.current);
    setToast(message);
    toastTimer.current = setTimeout(() => setToast(undefined), TOAST_MS);
  }, []);

  /** A 409 means the list on screen is stale: re-read it, then let the caller say so. */
  const rereadIfStale = useCallback(
    async (cause: unknown): Promise<never> => {
      if (isConcurrentChange(cause)) await reload().catch(() => undefined);
      throw cause;
    },
    [reload],
  );

  const send = useCallback(
    async (items: readonly TeachingLoadItem[], replace: boolean): Promise<void> => {
      setError(undefined);
      const sentFor = shownCycle.current;
      const result = await assignTeachingLoad(API_URL, { teacherId, items, replaceConflicts: replace }).catch(rereadIfStale);
      if (sentFor !== shownCycle.current) return;
      const done = result.created.length + result.reassigned.length;
      if (done > 0) say(assignedMessage(done));
      setRaw(result.conflicts);
      await reload().catch(() => setError("Se guardó, pero no pudimos releer la carga. Recarga la página."));
    },
    [teacherId, reload, say, rereadIfStale],
  );

  const assign = useCallback((items: TeachingLoadItem[]) => send(items, false), [send]);

  const markRemoving = useCallback((assignmentId: string, pending: boolean): void => {
    if (pending) inFlight.current.add(assignmentId);
    else inFlight.current.delete(assignmentId);
    setRemoving(new Set(inFlight.current));
  }, []);

  const freed = useCallback(
    (slot: LoadSlot): void => {
      drop(slot.assignmentId);
      clearTimeout(undoTimer.current);
      setUndoable(slot);
      undoTimer.current = setTimeout(() => setUndoable(undefined), UNDO_MS);
    },
    [drop],
  );

  /** A 404 means someone freed it first: the goal is met, so re-read quietly. */
  const removalFailed = useCallback(
    async (slot: LoadSlot, cause: unknown): Promise<void> => {
      if (isAlreadyGone(cause)) {
        drop(slot.assignmentId);
        await reload().catch(() => undefined);
        return;
      }
      await rereadIfStale(cause).catch((c: unknown) =>
        setError(describeLoadWriteError(c, `No se pudo quitar ${slot.subjectName} de ${slot.groupLabel}.`)),
      );
    },
    [drop, reload, rereadIfStale],
  );

  const remove = useCallback(
    (slot: LoadSlot): void => {
      if (inFlight.current.has(slot.assignmentId)) return;
      markRemoving(slot.assignmentId, true);
      setError(undefined);
      void deleteTeachingAssignment(API_URL, slot.assignmentId)
        .then(
          () => freed(slot),
          (cause: unknown) => removalFailed(slot, cause),
        )
        .finally(() => markRemoving(slot.assignmentId, false));
    },
    [markRemoving, freed, removalFailed],
  );

  const undo = useCallback(async (): Promise<void> => {
    if (undoable === undefined) return;
    clearTimeout(undoTimer.current);
    setUndoable(undefined);
    await send([{ groupId: undoable.groupId, subjectId: undoable.subjectId }], false).catch((cause: unknown) =>
      setError(describeLoadWriteError(cause, `No se pudo devolver ${undoable.subjectName} a ${undoable.groupLabel}.`)),
    );
  }, [undoable, send]);

  const replaceConflicts = useCallback(async (): Promise<void> => {
    const items = raw.map(({ groupId, subjectId }) => ({ groupId, subjectId }));
    await send(items, true).catch((cause: unknown) => setError(describeLoadWriteError(cause, "No se pudo reemplazar.")));
  }, [raw, send]);

  const keepConflicts = useCallback(() => setRaw([]), []);

  return { assign, remove, undo, replaceConflicts, keepConflicts, undoable, removing, conflicts: nameConflicts(raw, catalogue), toast, error };
}

function nameConflicts(conflicts: readonly TeachingLoadConflict[], catalogue: LoadCatalogue): NamedConflict[] {
  const groupLabel = new Map(catalogue.groups.map((g) => [g.id, g.label]));
  const subjectName = new Map(catalogue.subjects.map((s) => [s.id, s.name]));
  return conflicts.map((c) => ({
    key: `${c.groupId}|${c.subjectId}`,
    subjectName: subjectName.get(c.subjectId) ?? "Asignatura",
    groupLabel: groupLabel.get(c.groupId) ?? "Grupo",
    teacherName: c.teacherName,
  }));
}

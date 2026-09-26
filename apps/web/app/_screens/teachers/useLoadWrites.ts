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
/** Said when the bulk call only skipped: every picked group was already the teacher's. */
const ALREADY_ASSIGNED = "Esos grupos ya los tenía asignados.";

export interface LoadWrites {
  /** Sends slots for the teacher; throws when the request itself failed. */
  assign: (items: TeachingLoadItem[]) => Promise<void>;
  remove: (slot: LoadSlot) => void;
  undo: () => Promise<void>;
  replaceConflicts: () => Promise<void>;
  keepConflicts: () => void;
  /** Classes freed inside the current undo window; empty when there is none. */
  undoable: readonly LoadSlot[];
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

const NOTHING_TO_UNDO: readonly LoadSlot[] = [];

function restoreFailed(slots: readonly LoadSlot[]): string {
  const [only] = slots;
  if (slots.length === 1 && only !== undefined) return `No se pudo devolver ${only.subjectName} a ${only.groupLabel}.`;
  return `No se pudieron devolver las ${slots.length} clases.`;
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
  const [undoable, setUndoable] = useState<readonly LoadSlot[]>(NOTHING_TO_UNDO);
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
    setUndoable(NOTHING_TO_UNDO);
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
      else if (result.conflicts.length === 0 && result.skipped.length > 0) say(ALREADY_ASSIGNED);
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
      // Each removal joins the batch and restarts the window; when it runs
      // out the whole batch is kept as removed.
      clearTimeout(undoTimer.current);
      setUndoable((batch) => [...batch, slot]);
      undoTimer.current = setTimeout(() => setUndoable(NOTHING_TO_UNDO), UNDO_MS);
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
      // Like send(): an answer for a cycle no longer on screen must not arm
      // Deshacer or a banner on the one that is.
      const sentFor = shownCycle.current;
      const stillShown = (): boolean => sentFor === shownCycle.current;
      void deleteTeachingAssignment(API_URL, slot.assignmentId)
        .then(
          () => (stillShown() ? freed(slot) : undefined),
          (cause: unknown) => (stillShown() ? removalFailed(slot, cause) : undefined),
        )
        .finally(() => markRemoving(slot.assignmentId, false));
    },
    [markRemoving, freed, removalFailed],
  );

  const undo = useCallback(async (): Promise<void> => {
    if (undoable.length === 0) return;
    clearTimeout(undoTimer.current);
    setUndoable(NOTHING_TO_UNDO);
    const items = undoable.map(({ groupId, subjectId }) => ({ groupId, subjectId }));
    await send(items, false).catch((cause: unknown) => setError(describeLoadWriteError(cause, restoreFailed(undoable))));
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

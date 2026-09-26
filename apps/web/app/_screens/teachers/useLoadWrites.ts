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
  conflicts: readonly NamedConflict[];
  toast: string | undefined;
  error: string | undefined;
}

function assignedMessage(count: number): string {
  return count === 1 ? "1 grupo asignado" : `${count} grupos asignados`;
}

/**
 * The page's writes. A freed class is deleted AT ONCE (closing the tab must not
 * leave it assigned) and Deshacer gives it back through the bulk call; a slot
 * someone else holds comes back as a conflict the person resolves explicitly.
 */
export function useLoadWrites(
  teacherId: string,
  catalogue: LoadCatalogue,
  reload: () => Promise<void>,
  drop: (assignmentId: string) => void,
): LoadWrites {
  const [raw, setRaw] = useState<TeachingLoadConflict[]>([]);
  const [undoable, setUndoable] = useState<LoadSlot | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const undoTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => {
    clearTimeout(undoTimer.current);
    clearTimeout(toastTimer.current);
  }, []);

  const say = useCallback((message: string): void => {
    clearTimeout(toastTimer.current);
    setToast(message);
    toastTimer.current = setTimeout(() => setToast(undefined), TOAST_MS);
  }, []);

  const send = useCallback(
    async (items: readonly TeachingLoadItem[], replace: boolean): Promise<void> => {
      const result = await assignTeachingLoad(API_URL, { teacherId, items, replaceConflicts: replace });
      const done = result.created.length + result.reassigned.length;
      if (done > 0) say(assignedMessage(done));
      setRaw(result.conflicts);
      await reload().catch(() => setError("Se guardó, pero no pudimos releer la carga. Recarga la página."));
    },
    [teacherId, reload, say],
  );

  const assign = useCallback((items: TeachingLoadItem[]) => send(items, false), [send]);

  const remove = useCallback(
    (slot: LoadSlot): void => {
      setError(undefined);
      deleteTeachingAssignment(API_URL, slot.assignmentId)
        .then(() => {
          drop(slot.assignmentId);
          clearTimeout(undoTimer.current);
          setUndoable(slot);
          undoTimer.current = setTimeout(() => setUndoable(undefined), UNDO_MS);
        })
        .catch(() => setError(`No se pudo quitar ${slot.subjectName} de ${slot.groupLabel}. Intenta de nuevo.`));
    },
    [drop],
  );

  const undo = useCallback(async (): Promise<void> => {
    if (undoable === undefined) return;
    clearTimeout(undoTimer.current);
    setUndoable(undefined);
    await send([{ groupId: undoable.groupId, subjectId: undoable.subjectId }], false).catch(() =>
      setError(`No se pudo devolver ${undoable.subjectName} a ${undoable.groupLabel}. Agrégala de nuevo abajo.`),
    );
  }, [undoable, send]);

  const replaceConflicts = useCallback(async (): Promise<void> => {
    const items = raw.map(({ groupId, subjectId }) => ({ groupId, subjectId }));
    await send(items, true).catch(() => setError("No se pudo reemplazar. Intenta de nuevo."));
  }, [raw, send]);

  const keepConflicts = useCallback(() => setRaw([]), []);

  return { assign, remove, undo, replaceConflicts, keepConflicts, undoable, conflicts: nameConflicts(raw, catalogue), toast, error };
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

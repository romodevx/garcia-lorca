"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { API_URL } from "../../api-url";
import { addNote, editNote, listNotes, StudentsError, type StudentNote } from "../../students/api";

/**
 * Why a write was refused, in the words the school reads. `stale`: the note
 * changed, was voided or is gone since the list was read — re-read before retrying.
 */
export type NoteWriteResult = { ok: true } | { ok: false; reason: string; stale?: boolean };

/**
 * A pupil's notes, read once and kept here: the ONE source the three places
 * that show them read (the pupil's sheet, the list's modal and the payment
 * counter — T6). Each place mounts its own copy for its own pupil; all of them
 * read and write the same server history.
 */
export interface StudentNotes {
  /** Newest first, without the notes being voided right now (see hide). */
  readonly notes: readonly StudentNote[];
  /** True once the first read answered — before that, "no notes" is unknown. */
  readonly loaded: boolean;
  readonly problem: string | undefined;
  setProblem: (problem: string | undefined) => void;
  add: (body: string) => Promise<NoteWriteResult>;
  /** Replaces `note`'s text; refused if the note changed since it was read. */
  edit: (note: StudentNote, body: string) => Promise<NoteWriteResult>;
  /**
   * Takes notes out of the list while their void waits out its undo. A reload
   * in that time must not bring them back.
   */
  hide: (note: StudentNote) => void;
  /** Gives hidden notes back: the void was undone, or reached the server. */
  unhide: (notes: readonly StudentNote[], putBack: boolean) => void;
  reload: () => void;
}

const newestFirst = (a: StudentNote, b: StudentNote): number =>
  // By time, not by string: the server's timestamps vary in their fraction
  // digits, so "…:00Z" would sort after "…:00.5Z".
  Date.parse(b.createdAt) - Date.parse(a.createdAt);

function addRefusal(cause: unknown): string {
  return cause instanceof StudentsError && cause.isInvalid
    ? "La nota no es válida: escribe de 1 a 2000 caracteres."
    : "No se pudo guardar la nota. Intenta de nuevo.";
}

function editRefusal(cause: unknown): string {
  if (!(cause instanceof StudentsError)) return "No se pudo guardar la nota. Intenta de nuevo.";
  if (cause.status === 403) return "Solo quien escribió la nota, o la dirección, puede editarla.";
  if (cause.status === 404) return "Esta nota ya no existe: la anularon. Cancela para ver la lista actual.";
  if (cause.isConflict) return "La nota cambió o fue anulada mientras la editabas. Cancela para ver la versión actual.";
  if (cause.isInvalid) return "La nota no es válida: escribe de 1 a 2000 caracteres.";
  return "No se pudo guardar la nota. Intenta de nuevo.";
}

/**
 * Reads and writes a pupil's notes. `enabled` false reads nothing — for a
 * caller who may not read the pupil (the gateway would refuse it anyway).
 */
export function useStudentNotes(studentId: string, enabled = true): StudentNotes {
  const [notes, setNotes] = useState<StudentNote[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const hidden = useRef<Set<string>>(new Set());
  // Only the latest read may land: a slow answer for the previous pupil (or an
  // older reload) must not overwrite what the current one says.
  const latest = useRef(0);

  const reload = useCallback(() => {
    if (!enabled) return;
    const mine = ++latest.current;
    listNotes(API_URL, studentId)
      .then((rows) => {
        if (mine !== latest.current) return;
        setNotes(rows.filter((n) => !hidden.current.has(n.id)));
        setProblem(undefined);
      })
      .catch(() => {
        if (mine === latest.current) setProblem("No pudimos leer las notas.");
      })
      .finally(() => {
        if (mine === latest.current) setLoaded(true);
      });
  }, [studentId, enabled]);

  // Another pupil: nothing of the previous one stays on screen meanwhile.
  useEffect(() => {
    setNotes([]);
    setLoaded(false);
    setProblem(undefined);
    hidden.current = new Set();
    reload();
  }, [reload]);

  const add = useCallback(async (body: string): Promise<NoteWriteResult> => {
    try {
      const note = await addNote(API_URL, studentId, body);
      setNotes((prev) => [note, ...prev]);
      return { ok: true };
    } catch (cause) {
      return { ok: false, reason: addRefusal(cause) };
    }
  }, [studentId]);

  const edit = useCallback(async (original: StudentNote, body: string): Promise<NoteWriteResult> => {
    try {
      const note = await editNote(API_URL, studentId, original.id, body, original.body);
      setNotes((prev) => prev.map((n) => (n.id === note.id ? note : n)));
      return { ok: true };
    } catch (cause) {
      const stale = cause instanceof StudentsError && (cause.isConflict || cause.status === 404);
      return { ok: false, reason: editRefusal(cause), stale };
    }
  }, [studentId]);

  const hide = useCallback((note: StudentNote) => {
    hidden.current.add(note.id);
    setNotes((prev) => prev.filter((n) => n.id !== note.id));
  }, []);

  const unhide = useCallback((back: readonly StudentNote[], putBack: boolean) => {
    for (const n of back) hidden.current.delete(n.id);
    if (putBack) setNotes((prev) => [...prev, ...back].sort(newestFirst));
  }, []);

  return { notes, loaded, problem, setProblem, add, edit, hide, unhide, reload };
}

"use client";

import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { TextArea, useFocusFirstInvalid, useRequired } from "../../_design/form";
import { UndoToast } from "../../_design/UndoToast";
import { useUndoableRemoval } from "../../_design/useUndoableRemoval";
import { addNote, listNotes, StudentsError, voidNote, type StudentNote } from "../../students/api";

/** How long the author may take a note back (the server enforces it). */
const AUTHOR_VOID_WINDOW_MS = 15 * 60_000;
/**
 * The void reaches the server only when the 8-second undo runs out, so the
 * button leaves early enough for it to still land inside the window, with a
 * margin for the request itself.
 */
const VOID_LEAD_MS = 10_000;
/** How long after writing it the author is offered "Anular". */
const OWN_VOID_SPAN_MS = AUTHOR_VOID_WINDOW_MS - VOID_LEAD_MS;
const MAX_NOTE_CHARS = 2000;

/** The soonest moment, after `at`, at which one of `userId`'s notes stops being voidable. */
function earliestCutoff(notes: readonly StudentNote[], userId: string, at: number): number | undefined {
  let soonest: number | undefined;
  for (const note of notes) {
    if (note.voided || note.authorUserId !== userId) continue;
    const cutoff = Date.parse(note.createdAt) + OWN_VOID_SPAN_MS;
    if (cutoff >= at && (soonest === undefined || cutoff < soonest)) soonest = cutoff;
  }
  return soonest;
}
const NOTE_REQUIRED = ["body"] as const;

const WHEN = new Intl.DateTimeFormat("es-MX", { dateStyle: "medium", timeStyle: "short" });

export interface NotesPanelProps {
  studentId: string;
  /** The signed-in account: its own notes can be voided within the window. */
  userId: string;
  /** students:notes_write — may write notes (and void their own, briefly). */
  canWrite: boolean;
  /** students:notes_void — may void any note at any time. */
  canVoidAny: boolean;
  /** The clock; injected by tests. */
  now?: () => number;
}

/**
 * What staff wrote about a pupil, newest first.
 *
 * A note is never edited. A wrong one is voided: gone from this list at once,
 * with eight seconds to undo, and kept on the server marked "anulada" — which
 * only whoever may void any note (the owner) still sees.
 */
export function NotesPanel({ studentId, userId, canWrite, canVoidAny, now = Date.now }: NotesPanelProps): ReactElement {
  const [notes, setNotes] = useState<StudentNote[]>([]);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const form = useRequired(NOTE_REQUIRED);
  const [tooLong, setTooLong] = useState<string | undefined>(undefined);
  // Re-renders an open page at the exact moment the next of the author's own
  // voids stops being offered — not at a periodic re-check, which would leave
  // a gap in which a click is voided after the server's window.
  // Each tick re-arms: a timer that fires while the clock still reads before
  // the cutoff (early, or the wall clock stepped back) must not be the last.
  const [tick, setTick] = useState(0);
  const nextCutoff = canWrite && !canVoidAny ? earliestCutoff(notes, userId, now()) : undefined;
  useEffect(() => {
    if (nextCutoff === undefined) return;
    const timer = setTimeout(() => setTick((n) => n + 1), Math.max(0, nextCutoff - now()) + 1);
    return () => clearTimeout(timer);
  }, [nextCutoff, now, tick]);
  const removal = useUndoableRemoval<StudentNote>();
  const focusFirstInvalid = useFocusFirstInvalid();
  // A reload inside the grace period must not bring a voided note back.
  const pending = useRef<readonly StudentNote[]>([]);
  pending.current = removal.pending;

  const reload = useCallback(() => {
    listNotes(API_URL, studentId)
      .then((rows) => setNotes(rows.filter((n) => !pending.current.some((p) => p.id === n.id))))
      .catch(() => setProblem("No pudimos leer las notas."));
  }, [studentId]);

  useEffect(reload, [reload]);

  const add = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (saving) return;
    const complete = form.check({ body: draft });
    const long = [...draft.trim()].length > MAX_NOTE_CHARS;
    setTooLong(long ? `Una nota tiene como máximo ${MAX_NOTE_CHARS} caracteres; llevas ${[...draft.trim()].length}.` : undefined);
    if (!complete || long) {
      focusFirstInvalid();
      return;
    }
    setSaving(true);
    setProblem(undefined);
    try {
      const note = await addNote(API_URL, studentId, draft.trim());
      setNotes((prev) => [note, ...prev]);
      setDraft("");
    } catch (cause) {
      setProblem(
        cause instanceof StudentsError && cause.isInvalid
          ? "La nota no es válida: escribe de 1 a 2000 caracteres."
          : "No se pudo guardar la nota. Intenta de nuevo.",
      );
    } finally {
      setSaving(false);
    }
  };

  const mayVoid = (note: StudentNote): boolean => {
    if (note.voided) return false;
    if (canVoidAny) return true;
    return canWrite && note.authorUserId === userId && now() <= Date.parse(note.createdAt) + OWN_VOID_SPAN_MS;
  };

  const withdraw = (note: StudentNote): void => {
    // Re-checked on the click, not trusted from the last render: timers stop
    // while a laptop sleeps, so the button can outlive the author's window.
    if (!mayVoid(note)) {
      setTick((n) => n + 1);
      setProblem("Ya pasaron los 15 minutos: solo la dirección puede anular esta nota.");
      return;
    }
    setNotes((prev) => prev.filter((n) => n.id !== note.id));
    removal.remove(note, () => {
      voidNote(API_URL, studentId, note.id)
        .then(() => canVoidAny && reload())
        .catch(() => {
          setProblem("No se pudo anular la nota: pasaron los 15 minutos o ya estaba anulada.");
          reload();
        });
    });
  };

  return (
    <div style={{ display: "grid", gap: 14 }}>
      {canWrite && (
        <form onSubmit={(e) => void add(e)} style={{ display: "grid", gap: 10 }}>
          <TextArea
            label="Nueva nota"
            value={draft}
            onChange={setDraft}
            error={form.errors.body ?? tooLong}
            onClearError={() => {
              form.clear("body");
              setTooLong(undefined);
            }}
            help="Lo que el colegio debe recordar de este alumno. No se puede editar después; se puede anular en los primeros 15 minutos."
          />
          <div>
            <Btn type="submit" icon="plus" style={{ height: 44, fontSize: 14, opacity: saving ? 0.6 : 1 }}>Agregar nota</Btn>
          </div>
        </form>
      )}

      {notes.length === 0 ? (
        <p style={{ fontSize: 14, color: "var(--text-faint)" }}>Todavía no hay notas.</p>
      ) : (
        <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "grid", gap: 10 }}>
          {notes.map((note) => (
            <li
              key={note.id}
              style={{ padding: "12px 14px", borderRadius: 12, border: "1px solid var(--border)", background: note.voided ? "var(--surface-3)" : "var(--surface-2)" }}
            >
              <p style={{ fontSize: 14.5, color: "var(--text)", whiteSpace: "pre-wrap", textDecoration: note.voided ? "line-through" : "none" }}>{note.body}</p>
              <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap", marginTop: 6 }}>
                <span style={{ fontSize: 12.5, color: "var(--text-muted)" }}>
                  {note.authorName !== "" ? note.authorName : "Alguien del colegio"} · {WHEN.format(new Date(note.createdAt))}
                </span>
                {note.voided && (
                  <span style={{ padding: "2px 8px", borderRadius: 99, background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 12, fontWeight: 700 }}>Anulada</span>
                )}
                {mayVoid(note) && (
                  <Btn kind="ghost" onClick={() => withdraw(note)} aria-label="Anular nota" style={{ height: 36, padding: "0 12px", fontSize: 13.5, marginLeft: "auto" }}>
                    Anular
                  </Btn>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}

      {problem !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5 }}>{problem}</p>}
      {removal.latest !== undefined && (
        <UndoToast
          message="Se anuló la nota"
          onUndo={() => {
            const back = removal.undo() ?? [];
            // By time, not by string: the server's timestamps vary in their
            // fraction digits, so "…:00Z" would sort after "…:00.5Z".
            setNotes((prev) => [...prev, ...back].sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt)));
          }}
        />
      )}
    </div>
  );
}

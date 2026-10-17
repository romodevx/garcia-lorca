"use client";

import { useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { voidNote, type StudentNote } from "../../students/api";
import { Btn } from "../Controls";
import { TextArea, useFocusFirstInvalid, useRequired } from "../form";
import { UndoToast } from "../UndoToast";
import { useUndoableRemoval } from "../useUndoableRemoval";
import type { NoteWriteResult, StudentNotes } from "./useStudentNotes";

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
const NOTE_REQUIRED = ["body"] as const;

const WHEN = new Intl.DateTimeFormat("es-MX", { dateStyle: "medium", timeStyle: "short" });

const smallBtn = { height: 44, padding: "0 14px", fontSize: 13.5 } as const;

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

/** Too long, said on the field; undefined when it fits. */
function lengthError(text: string): string | undefined {
  const n = [...text.trim()].length;
  return n > MAX_NOTE_CHARS ? `Una nota tiene como máximo ${MAX_NOTE_CHARS} caracteres; llevas ${n}.` : undefined;
}

export interface StudentNotesPanelProps {
  /** The pupil's notes, from useStudentNotes — the one source (T6). */
  notes: StudentNotes;
  studentId: string;
  /** The signed-in account: its own notes can be edited, and voided within the window. */
  userId: string;
  /** students:notes_write — may write notes, edit their own, void their own briefly. */
  canWrite: boolean;
  /** students:notes_void — may void, and edit, any note at any time. */
  canVoidAny: boolean;
  /**
   * "always": the new-note field is open (the pupil's sheet, a modal opened to
   * write). "onDemand": it waits behind "Agregar nota" (beside a payment).
   */
  composer?: "always" | "onDemand";
  /** With composer "onDemand": open the field at once. */
  startComposing?: boolean;
  /**
   * Told whenever a note is being written or edited and not yet saved, so the
   * dialog around the panel stops closing on Esc or a stray click.
   */
  onDirtyChange?: (dirty: boolean) => void;
  /** The clock; injected by tests. */
  now?: () => number;
}

/**
 * What staff wrote about a pupil, newest first — the same panel in the pupil's
 * sheet, the list's notes modal and the payment counter.
 *
 * A note can be edited by its author (or the owner); the server keeps the text
 * each edit replaced, and the note shows who edited it last. A wrong note is
 * voided: gone from this list at once, with eight seconds to undo, and kept on
 * the server marked "anulada" — which only whoever may void any note sees.
 */
export function StudentNotesPanel({
  notes: state,
  studentId,
  userId,
  canWrite,
  canVoidAny,
  composer = "always",
  startComposing = false,
  onDirtyChange,
  now = Date.now,
}: StudentNotesPanelProps): ReactElement {
  const { notes, problem, setProblem } = state;
  const [composing, setComposing] = useState(composer === "always" || startComposing);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const form = useRequired(NOTE_REQUIRED);
  const [tooLong, setTooLong] = useState<string | undefined>(undefined);
  const [editing, setEditing] = useState<string | undefined>(undefined);
  const staleList = useRef(false);
  const composeRef = useRef<HTMLDivElement | null>(null);
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

  const dirty = draft.trim() !== "" || editing !== undefined;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  // Unmounting with a draft open must not leave the dialog believing it is dirty.
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);

  // Opening the field on demand puts the cursor in it: the click said "write".
  const [openedOnDemand, setOpenedOnDemand] = useState(composer === "onDemand" && startComposing);
  useEffect(() => {
    if (openedOnDemand) composeRef.current?.querySelector("textarea")?.focus();
  }, [openedOnDemand]);

  const add = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (saving) return;
    const complete = form.check({ body: draft });
    const long = lengthError(draft);
    setTooLong(long);
    if (!complete || long !== undefined) {
      focusFirstInvalid();
      return;
    }
    setSaving(true);
    setProblem(undefined);
    const result = await state.add(draft.trim());
    setSaving(false);
    if (!result.ok) {
      setProblem(result.reason);
      return;
    }
    setDraft("");
    if (composer === "onDemand") {
      setComposing(false);
      setOpenedOnDemand(false);
    }
  };

  const mayEdit = (note: StudentNote): boolean =>
    !note.voided && (canVoidAny || (canWrite && note.authorUserId === userId));

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
    state.hide(note);
    removal.remove(note, () => {
      voidNote(API_URL, studentId, note.id)
        .then(() => {
          state.unhide([note], false);
          if (canVoidAny) state.reload();
        })
        .catch(() => {
          setProblem("No se pudo anular la nota: pasaron los 15 minutos o ya estaba anulada.");
          state.unhide([note], false);
          state.reload();
        });
    });
  };

  return (
    <div style={{ display: "grid", gap: 14 }}>
      {canWrite && composing && (
        <form onSubmit={(e) => void add(e)} style={{ display: "grid", gap: 10 }}>
          <div ref={composeRef}>
            <TextArea
              label="Nueva nota"
              value={draft}
              onChange={setDraft}
              rows={4}
              error={form.errors.body ?? tooLong}
              onClearError={() => {
                form.clear("body");
                setTooLong(undefined);
              }}
              help="Lo que el colegio debe recordar de este alumno. Quien la escribe puede editarla después (el texto anterior queda guardado) y anularla en los primeros 15 minutos."
            />
          </div>
          <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
            <Btn type="submit" icon="plus" style={{ height: 44, fontSize: 14, opacity: saving ? 0.6 : 1 }}>Agregar nota</Btn>
            {composer === "onDemand" && (
              <Btn kind="ghost" type="button" style={smallBtn} onClick={() => { setComposing(false); setOpenedOnDemand(false); setDraft(""); form.clear("body"); setTooLong(undefined); }}>
                Cancelar
              </Btn>
            )}
          </div>
        </form>
      )}
      {canWrite && !composing && (
        <div>
          <Btn kind="ghost" type="button" icon="plus" style={{ height: 44, fontSize: 14 }} onClick={() => { setComposing(true); setOpenedOnDemand(true); }}>
            Agregar nota
          </Btn>
        </div>
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
              {editing === note.id ? (
                <NoteEditor
                  note={note}
                  onSave={async (body) => {
                    const result = await state.edit(note, body);
                    if (!result.ok && result.stale === true) staleList.current = true;
                    return result;
                  }}
                  onDone={() => {
                    setEditing(undefined);
                    // A refused edit said the list is out of date: re-read it, so
                    // the next edit starts from what the note says now.
                    if (staleList.current) {
                      staleList.current = false;
                      state.reload();
                    }
                  }}
                />
              ) : (
                <>
                  <p style={{ fontSize: 14.5, color: "var(--text)", whiteSpace: "pre-wrap", overflowWrap: "anywhere", textDecoration: note.voided ? "line-through" : "none" }}>{note.body}</p>
                  <NoteByline note={note} />
                  {(mayEdit(note) || mayVoid(note)) && (
                    <div style={{ display: "flex", gap: 8, justifyContent: "flex-end", marginTop: 6 }}>
                      {mayEdit(note) && (
                        <Btn kind="ghost" type="button" onClick={() => { setProblem(undefined); setEditing(note.id); }} aria-label="Editar nota" style={smallBtn}>
                          Editar
                        </Btn>
                      )}
                      {mayVoid(note) && (
                        <Btn kind="ghost" type="button" onClick={() => withdraw(note)} aria-label="Anular nota" style={smallBtn}>
                          Anular
                        </Btn>
                      )}
                    </div>
                  )}
                </>
              )}
            </li>
          ))}
        </ul>
      )}

      {problem !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5 }}>{problem}</p>}
      {removal.latest !== undefined && (
        <UndoToast
          message="Se anuló la nota"
          onUndo={() => state.unhide(removal.undo() ?? [], true)}
          onDismiss={removal.dismiss}
        />
      )}
    </div>
  );
}

/** Who wrote it and when; and, once edited, who edited it last and when. */
function NoteByline({ note }: { note: StudentNote }): ReactElement {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap", marginTop: 6 }}>
      <span style={{ fontSize: 12.5, color: "var(--text-muted)" }}>
        {note.authorName !== "" ? note.authorName : "Alguien del colegio"} · {WHEN.format(new Date(note.createdAt))}
      </span>
      {note.updatedAt !== undefined && (
        <span style={{ fontSize: 12.5, color: "var(--text-faint)" }}>
          Editada por {note.updatedByName !== undefined && note.updatedByName !== "" ? note.updatedByName : "alguien del colegio"} · {WHEN.format(new Date(note.updatedAt))}
        </span>
      )}
      {note.voided && (
        <span style={{ padding: "2px 8px", borderRadius: 99, background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 12, fontWeight: 700 }}>Anulada</span>
      )}
    </div>
  );
}

/**
 * One note's text, open for editing in its own place in the list. Its own
 * form: Enter in a textarea is a new line, the button saves. An edit that
 * changes nothing is not sent.
 */
function NoteEditor({ note, onSave, onDone }: {
  note: StudentNote;
  onSave: (body: string) => Promise<NoteWriteResult>;
  onDone: () => void;
}): ReactElement {
  const [text, setText] = useState(note.body);
  const [saving, setSaving] = useState(false);
  const [refusal, setRefusal] = useState<string | undefined>(undefined);
  const form = useRequired(NOTE_REQUIRED);
  const [tooLong, setTooLong] = useState<string | undefined>(undefined);
  const focusFirstInvalid = useFocusFirstInvalid();
  const box = useRef<HTMLFormElement | null>(null);

  useEffect(() => {
    box.current?.querySelector("textarea")?.focus();
  }, []);

  const save = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (saving) return;
    const complete = form.check({ body: text });
    const long = lengthError(text);
    setTooLong(long);
    if (!complete || long !== undefined) {
      focusFirstInvalid();
      return;
    }
    if (text.trim() === note.body) {
      onDone();
      return;
    }
    setSaving(true);
    setRefusal(undefined);
    const result = await onSave(text.trim());
    setSaving(false);
    if (result.ok) {
      onDone();
      return;
    }
    setRefusal(result.reason);
  };

  return (
    <form ref={box} onSubmit={(e) => void save(e)} style={{ display: "grid", gap: 10 }}>
      <TextArea
        label="Texto de la nota"
        value={text}
        onChange={setText}
        rows={4}
        error={form.errors.body ?? tooLong}
        onClearError={() => {
          form.clear("body");
          setTooLong(undefined);
        }}
      />
      {refusal !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5 }}>{refusal}</p>}
      <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
        <Btn type="submit" icon="check" style={{ height: 44, fontSize: 14, opacity: saving ? 0.6 : 1 }}>Guardar nota</Btn>
        <Btn kind="ghost" type="button" onClick={onDone} aria-label="Cancelar edición" style={smallBtn}>Cancelar</Btn>
      </div>
    </form>
  );
}

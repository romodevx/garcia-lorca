"use client";

import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { TextArea, useRequired } from "../../_design/form";
import { UndoToast } from "../../_design/UndoToast";
import { useUndoableRemoval } from "../../_design/useUndoableRemoval";
import { addNote, listNotes, voidNote, type StudentNote } from "../../students/api";

/** How long the author may take a note back (the server enforces it too). */
const AUTHOR_VOID_WINDOW_MS = 15 * 60_000;
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
  const removal = useUndoableRemoval<StudentNote>();
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
    if (saving || !form.check({ body: draft })) return;
    setSaving(true);
    setProblem(undefined);
    try {
      const note = await addNote(API_URL, studentId, draft.trim());
      setNotes((prev) => [note, ...prev]);
      setDraft("");
    } catch {
      setProblem("No se pudo guardar la nota. Intenta de nuevo.");
    } finally {
      setSaving(false);
    }
  };

  const mayVoid = (note: StudentNote): boolean => {
    if (note.voided) return false;
    if (canVoidAny) return true;
    const written = Date.parse(note.createdAt);
    return canWrite && note.authorUserId === userId && now() - written <= AUTHOR_VOID_WINDOW_MS;
  };

  const withdraw = (note: StudentNote): void => {
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
            error={form.errors.body}
            onClearError={() => form.clear("body")}
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
            setNotes((prev) => [...prev, ...back].sort((a, b) => b.createdAt.localeCompare(a.createdAt)));
          }}
        />
      )}
    </div>
  );
}

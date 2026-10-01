"use client";

import { useCallback, useState, type ReactElement, type ReactNode } from "react";

import { Btn } from "../Controls";
import { StudentNotesPanel } from "./StudentNotesPanel";
import { useStudentNotes, type StudentNotes } from "./useStudentNotes";

/** What the signed-in account may do with a pupil's notes. */
export interface NotesAccess {
  userId: string;
  /** students:read — reading the notes is reading the pupil. */
  canRead: boolean;
  /** students:notes_write. */
  canWrite: boolean;
  /** students:notes_void. */
  canVoidAny: boolean;
}

export interface NotesBeside {
  readonly notes: StudentNotes;
  readonly access: NotesAccess | undefined;
  /** Whether the notes panel is showing — the drawer widens to make room. */
  readonly wide: boolean;
  /** The field was opened from "Agregar nota" before there were notes. */
  readonly composing: boolean;
  readonly startComposing: () => void;
  /** A note is being written or edited and not saved: the drawer should not close on Esc. */
  readonly dirty: boolean;
  readonly setDirty: (dirty: boolean) => void;
}

/**
 * The state of a pupil's notes beside a payment form (T6). Once the panel has
 * shown it stays for the drawer's life: voiding the last note must not fold the
 * panel — and its undo — away under the cashier's hands.
 */
export function useNotesBeside(studentId: string, access: NotesAccess | undefined): NotesBeside {
  const notes = useStudentNotes(studentId, access?.canRead === true);
  const [composing, setComposing] = useState(false);
  const [shown, setShown] = useState(false);
  const [dirty, setDirty] = useState(false);
  const wanted = access?.canRead === true && (composing || notes.notes.length > 0);
  if (wanted && !shown) setShown(true);
  const startComposing = useCallback(() => setComposing(true), []);
  return { notes, access, wide: wanted || shown, composing, startComposing, dirty, setDirty };
}

export interface StudentNotesBesideProps {
  beside: NotesBeside;
  studentId: string;
  /**
   * The drawer's body has no padding of its own (the counter's form brings it):
   * the panel then keeps its own distance from the edge.
   */
  bare?: boolean;
  children: ReactNode;
}

/**
 * Registrar pago with the pupil's notes beside it (T6): a panel on the LEFT
 * when the pupil has notes; when not, an "Agregar nota" button that opens the
 * panel to write one. The same panel and the same source as the pupil's sheet
 * and the list's notes modal.
 *
 * The tree is the SAME in every state — the panel, the button and the warning
 * are slots that are empty or not, and the form always sits in the last one.
 * Wrapping the form differently once the notes arrive would remount it, and
 * the focus the drawer put on its first field would fall to <body>.
 *
 * The panel's field waits behind its button, so the counter's first field —
 * where the focus lands when the drawer opens — is still the payment's.
 * Narrower than a tablet, the panel stacks above the form (globals.css).
 */
export function StudentNotesBeside({ beside, studentId, bare = false, children }: StudentNotesBesideProps): ReactElement {
  const { access, notes } = beside;
  const ready = access !== undefined && access.canRead && notes.loaded;
  const wide = ready && beside.wide;
  const offerAdd = ready && !wide && access.canWrite;
  // A failed read is not "no notes": the cashier is told, not shown an empty side.
  const failed = ready && !wide && notes.problem !== undefined;
  const layout = wide ? "notes-beside" : "notes-beside-solo";

  return (
    <div className={bare ? `${layout} notes-beside-bare` : layout}>
      {wide ? (
        <aside aria-label="Notas del alumno" className="notes-beside-aside">
          <h3 style={{ fontSize: 15, fontWeight: 700, color: "var(--text)", marginBottom: 12 }}>Notas del alumno</h3>
          <StudentNotesPanel
            notes={notes}
            studentId={studentId}
            userId={access.userId}
            canWrite={access.canWrite}
            canVoidAny={access.canVoidAny}
            composer="onDemand"
            startComposing={beside.composing}
            onDirtyChange={beside.setDirty}
          />
        </aside>
      ) : null}
      {offerAdd ? (
        <div className="notes-beside-add">
          <Btn kind="ghost" type="button" icon="plus" style={{ height: 44, fontSize: 14 }} onClick={beside.startComposing}>
            Agregar nota
          </Btn>
        </div>
      ) : null}
      {failed ? (
        <p role="alert" className="notes-beside-add" style={{ color: "var(--bad-fg)", fontSize: 13.5 }}>{notes.problem}</p>
      ) : null}
      <div className="notes-beside-main">{children}</div>
    </div>
  );
}

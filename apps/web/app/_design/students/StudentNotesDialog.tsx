"use client";

import { useState, type ReactElement } from "react";

import { DrawerShell } from "../DrawerShell";
import { StudentNotesPanel } from "./StudentNotesPanel";
import { useStudentNotes } from "./useStudentNotes";

export interface StudentNotesDialogProps {
  studentId: string;
  /** The pupil's name as the list shows it (surnames first). */
  name: string;
  subtitle?: string;
  userId: string;
  canWrite: boolean;
  canVoidAny: boolean;
  onClose: () => void;
}

/**
 * A pupil's notes in a dialog, opened from a list without leaving it (T6). The
 * same panel and the same source as the pupil's sheet. The shell keeps the APG
 * dialog contract: focus in, trapped, and back to the trigger on close.
 */
export function StudentNotesDialog({ studentId, name, subtitle, userId, canWrite, canVoidAny, onClose }: StudentNotesDialogProps): ReactElement {
  const notes = useStudentNotes(studentId);
  // A note half written or half edited: Esc and the backdrop stop closing. The
  // ✕ still closes — a control that does not answer is worse.
  const [dirty, setDirty] = useState(false);
  return (
    <DrawerShell title={`Notas · ${name}`} subtitle={subtitle} icon="message" size="md" onClose={onClose} dismissible={!dirty}>
      <StudentNotesPanel
        notes={notes}
        studentId={studentId}
        userId={userId}
        canWrite={canWrite}
        canVoidAny={canVoidAny}
        onDirtyChange={setDirty}
      />
    </DrawerShell>
  );
}

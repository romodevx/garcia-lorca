"use client";

import { useRef, useState } from "react";

import { undoStudentImportFamilies, type StudentImportFamiliesUndo } from "../../studentImport/api";

export const UNDO_FAILED = "No pudimos deshacer las familias. Intenta de nuevo.";

export interface ImportFamiliesUndo {
  busy: boolean;
  /** The last undo's counts; stays until the next one answers (never by timer). */
  outcome: StudentImportFamiliesUndo | undefined;
  error: string | undefined;
  undo: () => Promise<void>;
}

/**
 * Undoing what an import's commit built. Reversible, so no confirmation; a
 * second press while one is in flight is ignored (a ref, not the state: the
 * same tick would not see it). The server is idempotent anyway.
 */
export function useImportFamiliesUndo(baseURL: string, batchId: string): ImportFamiliesUndo {
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<StudentImportFamiliesUndo | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const inFlight = useRef(false);

  const undo = async (): Promise<void> => {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError(undefined);
    try {
      setOutcome(await undoStudentImportFamilies(baseURL, batchId));
    } catch {
      setError(UNDO_FAILED);
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  };

  return { busy, outcome, error, undo };
}

"use client";

import { useRef, useState } from "react";

import { API_URL } from "../../api-url";
import { addStudentsToFamily, createFamily, familyRefusal } from "../../families/api";

/** A child just taken out of a family, still undoable. `dissolved`: it was the last one. */
export interface RemovedChild {
  familyId: string;
  familyName: string;
  id: string;
  name: string;
  dissolved: boolean;
}

export interface FamilyUndo {
  pending: RemovedChild | undefined;
  /** Why the last undo failed, for the screen to say. */
  error: string | undefined;
  offer: (removed: RemovedChild) => void;
  undo: () => Promise<void>;
  dismiss: () => void;
}

/**
 * The Undo of "Quitar de la familia", held by the Alumnos screen so it
 * outlives the family drawer (the toast never times out, house rule). Undo
 * re-adds the child; when that child was the last one the family dissolved,
 * and Undo builds it again with the same name (a new id).
 */
export function useFamilyUndo(onRestored: (removed: RemovedChild, familyId: string) => void): FamilyUndo {
  const [pending, setPending] = useState<RemovedChild | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const running = useRef(false);

  const undo = async (): Promise<void> => {
    const back = pending;
    if (back === undefined || running.current) return;
    running.current = true;
    setPending(undefined);
    setError(undefined);
    try {
      const familyId = back.dissolved
        ? (await createFamily(API_URL, { name: back.familyName, studentIds: [back.id] })).id
        : (await addStudentsToFamily(API_URL, back.familyId, [back.id])).id;
      onRestored(back, familyId);
    } catch (cause) {
      setError(familyRefusal(cause, `No se pudo regresar a ${back.name} a la familia.`));
    } finally {
      running.current = false;
    }
  };

  return {
    pending,
    error,
    offer: (removed) => { setError(undefined); setPending(removed); },
    undo,
    dismiss: () => setPending(undefined),
  };
}

/** What the toast says about the child just taken out. */
export function removedMessage(removed: RemovedChild): string {
  return removed.dissolved
    ? `Se quitó a ${removed.name}; la familia se quedó sin hijos y se disolvió`
    : `Se quitó a ${removed.name} de la familia`;
}

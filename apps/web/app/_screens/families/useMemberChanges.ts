"use client";

import { useState } from "react";

import { API_URL } from "../../api-url";
import { addStudentsToFamily, createFamily, familyRefusal, removeStudentFromFamily } from "../../families/api";

/** A child just taken out, still undoable. `dissolved`: it was the last one. */
export interface RemovedChild {
  id: string;
  name: string;
  dissolved: boolean;
}

export interface MemberChanges {
  removed: RemovedChild | undefined;
  /** The family is gone (its last child was removed); undo builds it again. */
  gone: boolean;
  error: string | undefined;
  remove: (id: string, name: string) => Promise<void>;
  undo: () => Promise<void>;
  dismissUndo: () => void;
  add: (id: string, name: string) => Promise<void>;
}

export interface MemberChangeHooks {
  familyName: string;
  /** The family or the list changed on the server. */
  onChanged: () => void;
  /** Undo rebuilt a dissolved family under a new id. */
  onReplaced: (familyId: string) => void;
  say: (message: string) => void;
}

/**
 * Taking a child out is done at once (the server keeps their parents'
 * links, ADR 0041) and offered back with Undo, which re-adds them — or, when
 * that was the last child and the family dissolved, builds it again.
 */
export function useMemberChanges(familyId: string, hooks: MemberChangeHooks): MemberChanges {
  const [removed, setRemoved] = useState<RemovedChild | undefined>(undefined);
  const [gone, setGone] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const remove = async (id: string, name: string): Promise<void> => {
    setError(undefined);
    try {
      const left = await removeStudentFromFamily(API_URL, familyId, id);
      setRemoved({ id, name, dissolved: left === null });
      if (left === null) setGone(true);
      hooks.onChanged();
    } catch (cause) {
      setError(familyRefusal(cause, `No se pudo quitar a ${name} de la familia.`));
    }
  };

  const undo = async (): Promise<void> => {
    const back = removed;
    if (back === undefined) return;
    setRemoved(undefined);
    try {
      if (back.dissolved) {
        const rebuilt = await createFamily(API_URL, { name: hooks.familyName, studentIds: [back.id] });
        setGone(false);
        hooks.onReplaced(rebuilt.id);
      } else {
        await addStudentsToFamily(API_URL, familyId, [back.id]);
      }
      hooks.onChanged();
    } catch (cause) {
      setError(familyRefusal(cause, `No se pudo regresar a ${back.name} a la familia.`));
    }
  };

  const add = async (id: string, name: string): Promise<void> => {
    setError(undefined);
    try {
      await addStudentsToFamily(API_URL, familyId, [id]);
      hooks.say(`${name} se agregó a la familia`);
      hooks.onChanged();
    } catch (cause) {
      setError(familyRefusal(cause, `No se pudo agregar a ${name}.`));
    }
  };

  return { removed, gone, error, remove, undo, dismissUndo: () => setRemoved(undefined), add };
}

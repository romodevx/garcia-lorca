"use client";

import { useRef, useState } from "react";

import { API_URL } from "../../api-url";
import { addStudentsToFamily, familyRefusal, removeStudentFromFamily } from "../../families/api";
import type { RemovedChild } from "./useFamilyUndo";

export interface MemberChanges {
  /** The family is gone (its last child was removed); the screen's Undo builds it again. */
  gone: boolean;
  error: string | undefined;
  /** Children whose removal is in flight: their button says so and ignores presses. */
  removing: ReadonlySet<string>;
  remove: (id: string, name: string) => Promise<void>;
  add: (id: string, name: string) => Promise<void>;
}

export interface MemberChangeHooks {
  familyName: string;
  /** The family or the list changed on the server. */
  onChanged: () => void;
  /** A child was taken out: the screen offers the Undo, which outlives this drawer. */
  onRemoved: (removed: RemovedChild) => void;
  say: (message: string) => void;
}

/**
 * Taking a child out is done at once (the server keeps their parents' links,
 * ADR 0041); the Undo lives on the screen, not here, so closing the drawer
 * does not take it away.
 */
export function useMemberChanges(familyId: string, hooks: MemberChangeHooks): MemberChanges {
  const [gone, setGone] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const inFlight = useRef(new Set<string>());
  const [removing, setRemoving] = useState<ReadonlySet<string>>(new Set());

  const remove = async (id: string, name: string): Promise<void> => {
    if (inFlight.current.has(id)) return;
    inFlight.current.add(id);
    setRemoving(new Set(inFlight.current));
    setError(undefined);
    try {
      const left = await removeStudentFromFamily(API_URL, familyId, id);
      if (left === null) setGone(true);
      hooks.onRemoved({ familyId, familyName: hooks.familyName, id, name, dissolved: left === null });
      hooks.onChanged();
    } catch (cause) {
      setError(familyRefusal(cause, `No se pudo quitar a ${name} de la familia.`));
    } finally {
      inFlight.current.delete(id);
      setRemoving(new Set(inFlight.current));
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

  return { gone, error, removing, remove, add };
}

import { useCallback, useMemo, useSyncExternalStore } from "react";

import {
  parseSelection,
  resolveSelection,
  selectionToSearch,
  type ResolvedSelection,
  type SelectionQuery,
  type TeachingOption,
} from "./selection";

/**
 * The query string as React state. The browser's own history is the store:
 * a choice is a pushState (so Back undoes it) and Back/Forward arrive as
 * popstate. Next's App Router supports native pushState and keeps its own
 * state in sync, so the pathname — the screen — is never touched.
 */
const listeners = new Set<() => void>();

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  window.addEventListener("popstate", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("popstate", listener);
  };
}

const currentSearch = (): string => window.location.search;
const serverSearch = (): string => "";

function pushSelection(query: SelectionQuery): void {
  window.history.pushState(null, "", `${window.location.pathname}${selectionToSearch(query)}`);
  for (const listener of listeners) listener();
}

const alwaysLeave = (): boolean => true;

export type PickerStep = "subject" | "group";

export interface TeachingSelection<T extends TeachingOption> extends ResolvedSelection<T> {
  /** Pick a subject by id. */
  chooseSubject: (subjectId: string) => void;
  /** Pick one of the options offered at the group step. */
  chooseGroup: (option: T) => void;
  /** Jump to any option at once (the "Cambiar" menu). */
  choose: (option: T) => void;
  /** Back to a step, leaving a history entry like any other move. */
  goTo: (step: PickerStep) => void;
}

/** The URL for an option: its subject and the class (or group), or just its group. */
function queryFor(option: TeachingOption, bySubject: boolean, keyByGroup: boolean): SelectionQuery {
  if (!bySubject) return { group: option.groupId };
  const subject = option.subjectId ?? "";
  return keyByGroup ? { subject, group: option.groupId } : { subject, assignment: option.id };
}

/** Where the teacher stands among `options`, kept in the URL. */
export function useTeachingSelection<T extends TeachingOption>(
  options: readonly T[],
  bySubject: boolean,
  keyByGroup = false,
  canLeave: () => boolean = alwaysLeave,
): TeachingSelection<T> {
  const search = useSyncExternalStore(subscribe, currentSearch, serverSearch);
  const resolved = useMemo(
    () => resolveSelection(options, parseSelection(search), bySubject, keyByGroup),
    [options, search, bySubject, keyByGroup],
  );
  const subjectId = resolved.subject?.id;

  // Every move away from the open list asks first when it has unsaved changes.
  const move = useCallback((query: SelectionQuery) => {
    if (canLeave()) pushSelection(query);
  }, [canLeave]);
  const chooseSubject = useCallback((next: string) => move({ subject: next }), [move]);
  const choose = useCallback((option: T) => move(queryFor(option, bySubject, keyByGroup)), [move, bySubject, keyByGroup]);
  const goTo = useCallback(
    (step: PickerStep) => move(step === "group" && bySubject ? { subject: subjectId } : {}),
    [move, bySubject, subjectId],
  );

  return { ...resolved, chooseSubject, chooseGroup: choose, choose, goTo };
}

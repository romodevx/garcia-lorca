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

/** The URL for an option: its subject and the class itself, or just its group. */
function queryFor(option: TeachingOption, bySubject: boolean): SelectionQuery {
  return bySubject ? { subject: option.subjectId ?? "", assignment: option.id } : { group: option.groupId };
}

/** Where the teacher stands among `options`, kept in the URL. */
export function useTeachingSelection<T extends TeachingOption>(options: readonly T[], bySubject: boolean): TeachingSelection<T> {
  const search = useSyncExternalStore(subscribe, currentSearch, serverSearch);
  const resolved = useMemo(() => resolveSelection(options, parseSelection(search), bySubject), [options, search, bySubject]);
  const subjectId = resolved.subject?.id;

  const chooseSubject = useCallback((next: string) => pushSelection({ subject: next }), []);
  const choose = useCallback((option: T) => pushSelection(queryFor(option, bySubject)), [bySubject]);
  const goTo = useCallback(
    (step: PickerStep) => pushSelection(step === "group" && bySubject ? { subject: subjectId } : {}),
    [bySubject, subjectId],
  );

  return { ...resolved, chooseSubject, chooseGroup: choose, choose, goTo };
}

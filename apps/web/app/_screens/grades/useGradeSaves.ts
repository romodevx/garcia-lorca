import { useEffect, useMemo, useSyncExternalStore } from "react";

import type { GradeSheetClient } from "../../grades/periodSheet";
import { GradeSaveStore } from "./gradeSaveStore";

/** Re-renders the caller whenever any cell's save changes. */
export function useSaveStoreVersion(store: GradeSaveStore): number {
  return useSyncExternalStore(store.subscribe, store.getVersion, store.getVersion);
}

/**
 * The capture screen's save store, owned ABOVE the keyed period/assignment
 * sheet so switching never drops a save in flight or an unsaved value, plus
 * the browser's "leave page?" prompt while anything is in flight or unsaved.
 */
export function useGradeSaves(client: GradeSheetClient): GradeSaveStore {
  const store = useMemo(() => new GradeSaveStore(client), [client]);
  useSaveStoreVersion(store);
  const guarding = store.pendingCount() + store.unsavedCount() > 0;

  useEffect(() => {
    if (!guarding) return;
    const guard = (event: BeforeUnloadEvent): void => {
      event.preventDefault();
      // Older browsers only prompt when returnValue is set.
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [guarding]);

  return store;
}

/** True when there is nothing unsaved to lose, or the user accepts leaving it for now. */
export function confirmLeavingUnsaved(unsaved: number): boolean {
  if (unsaved === 0) return true;
  const what = unsaved === 1 ? "1 calificación sin guardar" : `${unsaved} calificaciones sin guardar`;
  return window.confirm(`Hay ${what}. Se conservan para cuando regreses. ¿Cambiar de todos modos?`);
}

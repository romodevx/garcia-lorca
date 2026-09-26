import { useCallback, useEffect, useRef, useState } from "react";

const DISCARD_QUESTION = "Hay cambios sin guardar en la lista. ¿Salir sin guardar?";

/** Asks once, only when there is something to lose. */
export function confirmDiscard(dirty: boolean): boolean {
  return !dirty || window.confirm(DISCARD_QUESTION);
}

export interface LeaveGuard {
  /** The open list reports whether it has unsaved changes. */
  setDirty: (dirty: boolean) => void;
  /** True when leaving is fine (nothing unsaved, or the person confirmed). */
  canLeave: () => boolean;
}

/**
 * Keeps unsaved marks from being lost: every in-app way out (crumbs,
 * "Cambiar", "Volver") asks through `canLeave`, and closing or reloading the
 * tab gets the browser's own prompt. Nothing is asked when nothing changed.
 */
export function useLeaveGuard(): LeaveGuard {
  const [dirty, setDirtyState] = useState(false);
  const dirtyRef = useRef(false);

  const setDirty = useCallback((next: boolean) => {
    dirtyRef.current = next;
    setDirtyState(next);
  }, []);

  const canLeave = useCallback(() => {
    const ok = confirmDiscard(dirtyRef.current);
    if (ok) setDirty(false);
    return ok;
  }, [setDirty]);

  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent): void => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  return { setDirty, canLeave };
}

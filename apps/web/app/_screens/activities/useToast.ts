"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const TOAST_MS = 3000;
/** What an action's outcome toast lasts: long enough to read counts (ui-forms-and-modals: ≥ 8 s). */
export const LONG_TOAST_MS = 8000;

/** One toast at a time; a new one restarts the countdown instead of being cut short by the old one. */
export function useToast(): { toast: string | undefined; say: (message: string, ms?: number) => void } {
  const [toast, setToast] = useState<string | undefined>(undefined);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const say = useCallback((message: string, ms: number = TOAST_MS): void => {
    setToast(message);
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = setTimeout(() => setToast(undefined), ms);
  }, []);

  useEffect(() => () => {
    if (timer.current !== null) clearTimeout(timer.current);
  }, []);

  return { toast, say };
}

/** A done change and how to put it back (CC-210). */
export interface UndoOffer {
  message: string;
  restore: () => void;
}

/**
 * The house rule for a reversible removal: do it, then offer Deshacer — and
 * the offer does NOT time out. It stays until it is used or the next action
 * replaces it, so a person who looks away does not lose the way back.
 */
export function useUndoOffer(): { offer: UndoOffer | undefined; offerUndo: (offer: UndoOffer) => void; withdraw: () => void } {
  const [offer, setOffer] = useState<UndoOffer | undefined>(undefined);
  const withdraw = useCallback((): void => setOffer(undefined), []);
  return { offer, offerUndo: setOffer, withdraw };
}

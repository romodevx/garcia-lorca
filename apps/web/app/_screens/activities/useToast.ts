"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const TOAST_MS = 3000;

/** One toast at a time; a new one restarts the countdown instead of being cut short by the old one. */
export function useToast(): { toast: string | undefined; say: (message: string) => void } {
  const [toast, setToast] = useState<string | undefined>(undefined);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const say = useCallback((message: string): void => {
    setToast(message);
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = setTimeout(() => setToast(undefined), TOAST_MS);
  }, []);

  useEffect(() => () => {
    if (timer.current !== null) clearTimeout(timer.current);
  }, []);

  return { toast, say };
}

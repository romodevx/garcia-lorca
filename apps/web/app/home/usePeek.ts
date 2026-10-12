import { useCallback, useEffect, useState, type KeyboardEvent } from "react";

/**
 * Which card's shortcuts are open (PLAN-redesign-home §3.2): one at most, and a
 * press anywhere outside that card closes them. Hover never goes through here —
 * the hover peek is CSS only.
 */
export interface Peek {
  openId: string | null;
  toggle: (id: string) => void;
  close: () => void;
}

/** The attribute a card carries so a press inside it does not count as "outside". */
export const PEEK_OWNER_ATTR = "data-peek-owner";

export function usePeek(): Peek {
  const [openId, setOpenId] = useState<string | null>(null);

  useEffect(() => {
    if (openId === null) return;
    const closeIfOutside = (event: PointerEvent): void => {
      const target = event.target instanceof Element ? event.target : null;
      if (target?.closest(`[${PEEK_OWNER_ATTR}]`)?.getAttribute(PEEK_OWNER_ATTR) !== openId) setOpenId(null);
    };
    document.addEventListener("pointerdown", closeIfOutside);
    return () => document.removeEventListener("pointerdown", closeIfOutside);
  }, [openId]);

  const toggle = useCallback((id: string) => setOpenId((current) => (current === id ? null : id)), []);
  const close = useCallback(() => setOpenId(null), []);
  return { openId, toggle, close };
}

const NEXT_KEYS = new Set(["ArrowDown", "ArrowRight"]);
const PREVIOUS_KEYS = new Set(["ArrowUp", "ArrowLeft"]);

/** Arrows (wrapping), Home and End move the focus between a peek's shortcuts; Tab still works too. */
export function moveFocusAmongShortcuts(event: KeyboardEvent<HTMLElement>): void {
  const buttons = [...event.currentTarget.querySelectorAll<HTMLButtonElement>("button")];
  const count = buttons.length;
  const at = buttons.findIndex((b) => b === document.activeElement);
  let next: number | undefined;
  if (NEXT_KEYS.has(event.key)) next = (at + 1) % count;
  else if (PREVIOUS_KEYS.has(event.key)) next = (at - 1 + count) % count;
  else if (event.key === "Home") next = 0;
  else if (event.key === "End") next = count - 1;
  if (next === undefined) return;
  event.preventDefault();
  buttons[next]?.focus();
}

"use client";

import { useEffect, useRef } from "react";

/**
 * The school's session rule: keep it open while somebody is working, and close
 * it after fifteen minutes of nobody touching it.
 *
 * It exists because the panel runs on a computer at the front desk. Somebody
 * walks away to attend a parent, and the next person who sits down must not
 * find an open session holding the school's money.
 *
 * Two halves, and both are needed:
 *
 *  - The WATCHDOG ends the session at fifteen idle minutes. Real activity —
 *    a click, a key, a scroll — pushes it back.
 *  - The KEEP-ALIVE renews the session every ten minutes WHILE somebody is
 *    active, so a cashier entering payments for an hour never meets a login
 *    screen mid-entry. It stops the moment the watchdog fires, or the panel
 *    would keep an empty office logged in forever.
 *
 * The browser is not the enforcement — the server refuses a staff session that
 * has sat unused for fifteen minutes, and the cookie expires with it. This is
 * what makes the rule visible and immediate rather than a surprise on the next
 * click.
 */

/** How long the panel may sit untouched. The school's own number. */
export const IDLE_LIMIT_MS = 15 * 60 * 1000;

/**
 * How often an ACTIVE session renews itself. Comfortably inside the idle limit,
 * so the token is always renewed well before it could lapse.
 */
export const KEEP_ALIVE_MS = 10 * 60 * 1000;

/** What counts as somebody being there. */
const ACTIVITY_EVENTS = ["click", "keydown", "mousemove", "scroll", "touchstart"] as const;

export interface SessionTimeoutOptions {
  /** Called once, when the session has been idle too long. */
  onExpire: () => void;
  /** Called periodically while somebody is using the panel. */
  onKeepAlive: () => void;
}

export function useSessionTimeout({ onExpire, onKeepAlive }: SessionTimeoutOptions): void {
  // Held in refs so re-renders never restart the clock: a panel that re-rendered
  // on every keystroke would reset its own watchdog and never expire.
  const lastActivity = useRef(Date.now());
  const lastRenewal = useRef(Date.now());
  const expired = useRef(false);
  const expire = useRef(onExpire);
  const keepAlive = useRef(onKeepAlive);
  expire.current = onExpire;
  keepAlive.current = onKeepAlive;

  useEffect(() => {
    const touch = (): void => {
      // After expiry, activity means nothing: the session is over and only a
      // fresh login brings it back.
      if (!expired.current) {
        lastActivity.current = Date.now();
      }
    };
    for (const event of ACTIVITY_EVENTS) {
      document.addEventListener(event, touch, { passive: true });
    }

    // One timer for both jobs, checked often enough to be accurate to the
    // second and rarely enough to cost nothing.
    const tick = setInterval(() => {
      if (expired.current) {
        return;
      }
      const idleFor = Date.now() - lastActivity.current;
      if (idleFor >= IDLE_LIMIT_MS) {
        expired.current = true;
        expire.current();
        return;
      }
      // Renewed only if somebody actually DID something since the last
      // renewal. Renewing on the mere passing of time would keep an empty
      // office logged in on the server, which is the thing this exists to stop.
      const usedSinceLastRenewal = lastActivity.current > lastRenewal.current;
      if (usedSinceLastRenewal && Date.now() - lastRenewal.current >= KEEP_ALIVE_MS) {
        lastRenewal.current = Date.now();
        keepAlive.current();
      }
    }, 1000);

    return () => {
      clearInterval(tick);
      for (const event of ACTIVITY_EVENTS) {
        document.removeEventListener(event, touch);
      }
    };
  }, []);
}

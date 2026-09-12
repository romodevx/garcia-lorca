"use client";

import { useEffect, useRef } from "react";

/**
 * Keeps a session that IS being used from lapsing under the person using it.
 *
 * The server grants the panel a day of not being opened (iam's staffRefreshTTL,
 * mirrored by the gl_refresh cookie's Max-Age). Inside that day the access
 * token is short, so a panel somebody works in for hours would otherwise keep
 * meeting 401s; this renews it every ten minutes WHILE there is somebody there,
 * so the token is always rotated well before it could lapse.
 *
 * WHAT USED TO BE HERE, AND WHY IT IS NOT: a watchdog that ended the session at
 * fifteen idle minutes, for the computer on the shared front desk — so the next
 * person to sit down would not find the school's money open. The school weighed
 * that against being thrown out while working, and against closing the browser
 * and having to sign in again, and chose the day. Their call, and it costs them
 * that desk.
 *
 * What is left is only a renewal, and it is NOT a security boundary — it never
 * was. The server is: it refuses a staff session that has sat unused for a day,
 * and the cookie expires with it. Nothing the browser does or fails to do
 * extends that.
 */

/**
 * How often an ACTIVE session renews itself. Comfortably inside the access
 * token's own life, so the token is always renewed before it could lapse.
 */
export const KEEP_ALIVE_MS = 10 * 60 * 1000;

/** What counts as somebody being there. */
const ACTIVITY_EVENTS = ["click", "keydown", "mousemove", "scroll", "touchstart"] as const;

export interface SessionKeepAliveOptions {
  /** Called periodically while somebody is using the panel. */
  onKeepAlive: () => void;
}

export function useSessionKeepAlive({ onKeepAlive }: SessionKeepAliveOptions): void {
  // Held in refs so re-renders never restart the clock: a panel that re-rendered
  // on every keystroke would reset its own timer and never renew.
  const lastActivity = useRef(Date.now());
  const lastRenewal = useRef(Date.now());
  const keepAlive = useRef(onKeepAlive);
  keepAlive.current = onKeepAlive;

  useEffect(() => {
    const touch = (): void => {
      lastActivity.current = Date.now();
    };
    for (const event of ACTIVITY_EVENTS) {
      document.addEventListener(event, touch, { passive: true });
    }

    const tick = setInterval(() => {
      // Renewed only if somebody actually DID something since the last
      // renewal. Renewing on the mere passing of time would hold the session
      // open on the server for an office nobody is in.
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

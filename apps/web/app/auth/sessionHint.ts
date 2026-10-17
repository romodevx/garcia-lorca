/**
 * Whether booting should ask the gateway for a session at all (T13).
 *
 * Without a session cookie the answer is known — no session — and asking costs
 * two 401s the browser logs as errors. The web server's session hint says if a
 * cookie is there (app/api/v1/auth/session-hint/route.ts). It lives under the
 * refresh cookie's path: an access cookie lasts minutes, so a returning visitor
 * often holds only gl_refresh, and the browser sends it nowhere else. Only a
 * clear "none" skips the probes: any doubt (a failure, an odd answer) asks the
 * gateway as before, so a live session is never sent to the login screen.
 */
import { REFRESH_COOKIE_PATH } from "./cookiePaths";

export const SESSION_HINT_PATH = `${REFRESH_COOKIE_PATH}/session-hint`;

export async function mayHaveSession(): Promise<boolean> {
  try {
    const res = await fetch(SESSION_HINT_PATH, { method: "GET", credentials: "same-origin", cache: "no-store" });
    if (!res.ok) return true;
    const body = (await res.json()) as { present?: unknown };
    return body.present !== false;
  } catch {
    return true;
  }
}

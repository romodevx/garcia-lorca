import { pathToView, type View } from "./routes";

/**
 * Where somebody was going when a direct link found no live session (CC-309).
 *
 * It lives in sessionStorage, not in a ref: in the browser, /horarios → /entrar
 * changes the value of the [[...slug]] catch-all and Next mounts a NEW page, so
 * whatever the first AppShell kept in memory is gone by the time the person
 * signs in. sessionStorage is per tab, never in the address bar (a `?next=`
 * would be copyable and forgeable) and dies with the tab.
 *
 * Only an address of this site is ever stored, and it is resolved through
 * routes.ts again when read, so a planted value can't send anyone elsewhere.
 */

const STORAGE_KEY = "gl.pendingDestination";

/** Screens that are where a sign-in lands anyway, or that a sign-in must never land on. */
const NOT_A_DESTINATION: ReadonlySet<View["screen"]> = new Set(["login", "dashboard", "changePassword"]);

/**
 * A same-site path: one leading slash and nothing a browser would read as
 * another host ("//x", "/\x"), no scheme, no control characters.
 */
export function isInternalPath(path: string): boolean {
  if (!path.startsWith("/")) return false;
  if (path.startsWith("//") || path.startsWith("/\\")) return false;
  // eslint-disable-next-line no-control-regex -- refusing control characters is the point
  return !/[\u0000-\u001f\u007f]/.test(path);
}

function destinationOf(path: string | null): View | undefined {
  if (path === null || !isInternalPath(path)) return undefined;
  const view = pathToView(path);
  return NOT_A_DESTINATION.has(view.screen) ? undefined : view;
}

function storage(): Storage | undefined {
  try {
    return typeof window === "undefined" ? undefined : window.sessionStorage;
  } catch {
    // Blocked site data: the person still signs in, they just land on Inicio.
    return undefined;
  }
}

/** Remembers `path` for after the sign-in, when it is a real destination of this site. */
export function rememberPendingDestination(path: string): void {
  if (destinationOf(path) === undefined) return;
  try {
    storage()?.setItem(STORAGE_KEY, path);
  } catch {
    // Full or blocked storage: losing the destination is the safe failure.
  }
}

/** Drops whatever was remembered: a forced password change, or another person in the same tab. */
export function forgetPendingDestination(): void {
  try {
    storage()?.removeItem(STORAGE_KEY);
  } catch {
    // Nothing to forget when storage is unavailable.
  }
}

/** The remembered destination, at most once. */
export function takePendingDestination(): View | undefined {
  let stored: string | null = null;
  try {
    stored = storage()?.getItem(STORAGE_KEY) ?? null;
  } catch {
    return undefined;
  }
  forgetPendingDestination();
  return destinationOf(stored);
}

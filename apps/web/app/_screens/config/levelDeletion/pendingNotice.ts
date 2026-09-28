/**
 * The sentence a finished level deletion leaves for Configuración.
 *
 * It lives at module scope, not in a component's state, because Next REMOUNTS
 * <AppShell> on every route change (the whole shell is one [[...slug]] page):
 * state set on the deletion page dies with it before /configuracion paints.
 * Module scope survives client-side navigation and is gone on a full reload,
 * which is exactly the lifetime of "you just deleted a level".
 */
let pending: string | undefined;

/** Leaves the notice for the next Configuración screen to show. */
export function leaveLevelNotice(message: string): void {
  pending = message;
}

/** The notice waiting to be shown, if any (does not consume it). */
export function peekLevelNotice(): string | undefined {
  return pending;
}

/** Marks the notice as delivered: it is said once, not on every later visit. */
export function clearLevelNotice(): void {
  pending = undefined;
}

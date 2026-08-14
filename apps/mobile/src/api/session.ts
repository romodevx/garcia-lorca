/**
 * Dependency-free pub/sub for the "session lost" signal.
 *
 * `emitSessionLost` fires at the one true dead-session moment: when the gateway
 * rejects a real refresh token (see `auth.ts` `refresh`). The app root subscribes
 * via `onSessionLost` to force navigation back to Login — so a dead session can
 * never leave the app in a zombie authenticated-looking state (bug_007).
 */

type SessionLostListener = () => void;

const listeners = new Set<SessionLostListener>();

/** Subscribes to the session-lost signal; returns an unsubscribe function. */
export function onSessionLost(listener: SessionLostListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Notifies every subscriber that the session is dead (the refresh was rejected). */
export function emitSessionLost(): void {
  for (const listener of listeners) {
    listener();
  }
}

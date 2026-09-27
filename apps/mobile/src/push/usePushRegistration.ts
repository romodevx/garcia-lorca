import { useCallback, useEffect, useRef } from "react";
import { useFreshDataSignal } from "../state/freshness";
import { loadNotifications } from "./notificationsModule";
import { registerForPush, type PushRegistration } from "./register";
import { unregisterPush } from "./unregister";

/** Outcomes that end registration for this session; only "failed" is retried. */
const SETTLED: ReadonlySet<PushRegistration> = new Set(["registered", "denied", "no-project", "unsupported"]);

/**
 * Keeps this phone registered for pushes while a family is signed in:
 *  - registers once per session; a "failed" attempt is retried the next time
 *    the app comes back to the foreground (the freshness signal);
 *  - re-registers whenever the push service rotates the device token.
 * One attempt runs at a time; a rotation that lands mid-attempt runs right after.
 *
 * Every sign-in / sign-out starts a new session. An attempt that finishes in a
 * session other than the one it started in never settles the new one: if it
 * saved a token, that token belongs to a family that has left, so it is
 * unregistered at once (queued as a pending delete when the Bearer is already
 * gone), and a family signed in meanwhile gets its own attempt.
 */
export function usePushRegistration(signedIn: boolean): void {
  const settled = useRef(false);
  const inFlight = useRef(false);
  const rerun = useRef(false);
  const session = useRef(0);
  const signedInRef = useRef(signedIn);
  signedInRef.current = signedIn;

  const attempt = useCallback(async (force: boolean): Promise<void> => {
    if (!signedInRef.current || (settled.current && !force)) {
      return;
    }
    if (inFlight.current) {
      rerun.current ||= force;
      return;
    }
    inFlight.current = true;
    const startedIn = session.current;
    let outcome: PushRegistration;
    try {
      outcome = await registerForPush();
    } finally {
      inFlight.current = false;
    }
    const sameSession = startedIn === session.current && signedInRef.current;
    if (sameSession) {
      settled.current = SETTLED.has(outcome);
    } else if (outcome === "registered") {
      await unregisterPush();
    }
    const forceRerun = rerun.current;
    rerun.current = false;
    if (forceRerun || !sameSession) {
      await attempt(forceRerun);
    }
  }, []);

  useEffect(() => {
    session.current += 1;
    settled.current = false;
    if (!signedIn) {
      return;
    }
    void attempt(false);
  }, [signedIn, attempt]);

  const retryIfUnsettled = useCallback(() => attempt(false), [attempt]);
  useFreshDataSignal(retryIfUnsettled, signedIn);

  useEffect(() => {
    const notifications = loadNotifications();
    if (notifications === null) {
      return;
    }
    const subscription = notifications.addPushTokenListener(() => {
      void attempt(true);
    });
    return () => subscription.remove();
  }, [attempt]);
}

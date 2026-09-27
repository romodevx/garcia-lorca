import { useCallback, useEffect, useRef } from "react";
import { useFreshDataSignal } from "../state/freshness";
import { loadNotifications } from "./notificationsModule";
import { registerForPush, type PushRegistration } from "./register";

/** Outcomes that end registration for this session; only "failed" is retried. */
const SETTLED: ReadonlySet<PushRegistration> = new Set(["registered", "denied", "no-project", "unsupported"]);

/**
 * Keeps this phone registered for pushes while a family is signed in:
 *  - registers once per session; a "failed" attempt is retried the next time
 *    the app comes back to the foreground (the freshness signal);
 *  - re-registers whenever the push service rotates the device token.
 * One attempt runs at a time; a rotation that lands mid-attempt runs right after.
 */
export function usePushRegistration(signedIn: boolean): void {
  const settled = useRef(false);
  const inFlight = useRef(false);
  const rerun = useRef(false);
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
    try {
      settled.current = SETTLED.has(await registerForPush());
    } finally {
      inFlight.current = false;
    }
    if (rerun.current) {
      rerun.current = false;
      await attempt(true);
    }
  }, []);

  useEffect(() => {
    if (!signedIn) {
      settled.current = false;
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

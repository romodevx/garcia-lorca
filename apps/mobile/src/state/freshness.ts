import { useEffect, useRef } from "react";
import { AppState as RNAppState, type AppStateStatus } from "react-native";

/**
 * "The data on screen may be stale — reload it." One app-wide signal, raised
 * when the app returns to the foreground or an attendance push arrives, and
 * heard by every data hook that shows something a teacher can change while the
 * phone sits in a pocket (attendance, the inbox). A plain listener set: there is
 * no state to hold, only a moment to announce.
 */

type Reload = () => Promise<void>;

const listeners = new Set<Reload>();

/** Asks every subscribed resource to reload. Failures stay inside each resource. */
export function requestFreshData(): void {
  for (const reload of listeners) {
    void reload().catch(() => undefined);
  }
}

/** Subscribes `reload` to the freshness signal for the component's lifetime. */
export function useFreshDataSignal(reload: Reload): void {
  const latest = useRef(reload);
  latest.current = reload;
  useEffect(() => {
    const listener: Reload = () => latest.current();
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  }, []);
}

/**
 * Raises the freshness signal whenever the app comes back to the foreground —
 * the parent who opens the app after the teacher saved sees today's record, not
 * the one from this morning. Mount once, at the root.
 */
export function useForegroundRefresh(): void {
  const previous = useRef<AppStateStatus | null>(RNAppState.currentState);
  useEffect(() => {
    const subscription = RNAppState.addEventListener("change", (next) => {
      const cameBack = next === "active" && previous.current !== "active";
      previous.current = next;
      if (cameBack) {
        requestFreshData();
      }
    });
    return () => subscription.remove();
  }, []);
}

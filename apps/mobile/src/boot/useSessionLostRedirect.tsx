import { router } from "expo-router";
import { useEffect } from "react";
import { onSessionLost } from "@/src/api";
import { useAppState } from "@/src/state/AppState";

/**
 * Subscribes once to the "session lost" signal (a refresh token the gateway
 * rejected — see api/session) and forces the user back to the login screen,
 * resetting the in-memory notifications so the next session starts clean.
 *
 * This closes the zombie signed-in state (bug_007) where the chrome still looked
 * authenticated after the session had actually died. Mounted inside the router +
 * provider context by the root layout; the subscription is set up once and torn
 * down on unmount.
 */
export function useSessionLostRedirect(): void {
  const { refreshNotifications } = useAppState();
  useEffect(() => {
    return onSessionLost(() => {
      router.replace("/");
      // Tokens are already cleared by refresh(), so this no-ops to an empty inbox.
      void refreshNotifications();
    });
  }, [refreshNotifications]);
}

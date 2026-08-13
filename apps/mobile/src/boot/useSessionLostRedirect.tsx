import { router } from "expo-router";
import { useEffect } from "react";
import { onSessionLost } from "@/src/api";
import { useAppState } from "@/src/state/AppState";

/**
 * Subscribes once to the "session lost" signal (a refresh token the gateway
 * rejected — see api/session) and forces the user back to the login screen,
 * resetting BOTH in-memory family slices so the next guardian on a shared device
 * starts clean.
 *
 * This closes the zombie signed-in state (bug_007) where the chrome still looked
 * authenticated after the session had actually died. It also clears the previous
 * guardian's child slice (bug_006): resetting only notifications left
 * `children`/`selectedChildId` holding the prior guardian's data. Mounted inside
 * the router + provider context by the root layout; the subscription is set up
 * once and torn down on unmount.
 */
export function useSessionLostRedirect(): void {
  const { refreshChildren, refreshNotifications } = useAppState();
  useEffect(() => {
    return onSessionLost(() => {
      router.replace("/");
      // Tokens are already cleared by refresh(), so both short-circuit to an empty
      // slice without firing a request.
      void refreshChildren();
      void refreshNotifications();
    });
  }, [refreshChildren, refreshNotifications]);
}

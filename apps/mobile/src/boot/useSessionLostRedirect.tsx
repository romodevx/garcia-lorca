import { router } from "expo-router";
import { useEffect } from "react";
import { onSessionLost } from "@/src/api";
import { SESSION_ENDED_ROUTE } from "@/src/navigation/routes";
import { unregisterPush } from "@/src/push/unregister";
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
 * once and torn down on unmount. The phone's push token is unregistered too,
 * best-effort: the session is already gone, so it becomes a pending delete
 * retried before the next registration.
 */
export function useSessionLostRedirect(): void {
  const { refreshChildren, refreshNotifications } = useAppState();
  useEffect(() => {
    return onSessionLost(() => {
      // The login screen, saying why: a family that did not sign out must not
      // be left guessing (ISSUE-T14-102).
      router.replace(SESSION_ENDED_ROUTE);
      void unregisterPush();
      // Tokens are already cleared by refresh(), so both short-circuit to an empty
      // slice without firing a request.
      void refreshChildren();
      void refreshNotifications();
    });
  }, [refreshChildren, refreshNotifications]);
}

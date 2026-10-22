import { router } from "expo-router";
import { useEffect } from "react";
import { getAccessToken } from "@/src/api";
import { useCurrentHref } from "@/src/navigation/href";
import { LOGIN_ROUTE, RETURN_TO_PARAM } from "@/src/navigation/routes";

/**
 * Keeps every route but the login screen behind a session.
 *
 * A deep link can open the app straight on any route. Without a session the
 * family used to land on that screen, empty and signed out. Now they are sent
 * to the login screen, which remembers the route (and its query) and opens it once they sign
 * in (see destinationAfterSignIn).
 *
 * The navigator must still mount on the first render, so this redirects from an
 * effect instead of withholding the Stack. Mounted by the root layout inside
 * the router context; the token cache is already hydrated by then.
 */
export function useRequireSession(): void {
  const { pathname, href } = useCurrentHref();
  useEffect(() => {
    if (pathname === LOGIN_ROUTE || getAccessToken() !== null) {
      return;
    }
    // The whole href, query included: a boleta link carries its studentId there.
    router.replace({ pathname: LOGIN_ROUTE, params: { [RETURN_TO_PARAM]: href } });
  }, [pathname, href]);
}

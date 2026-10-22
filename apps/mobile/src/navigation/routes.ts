/**
 * The hrefs that move a family across the session boundary.
 *
 * The Inicio tab is `(tabs)/inicio`, not `(tabs)/index`, on purpose: with an
 * index inside the group, "/" named BOTH the login screen and Inicio, and
 * expo-router resolved it to the tab next to wherever the family already was —
 * so a dead session "returned to login" by landing on an empty Inicio
 * (ISSUE-T14-102). "/" now means the login screen, unambiguously.
 */
export const LOGIN_ROUTE = "/";
export const HOME_ROUTE = "/inicio";

/**
 * Where a family was going when the login screen interrupted them (a deep link
 * opened without a session). Read only through destinationAfterSignIn.
 */
export const RETURN_TO_PARAM = "destino";

/** Why the login screen is showing, when it was not the family's own choice. */
export const SIGN_IN_REASON_PARAM = "motivo";
export const SESSION_ENDED_REASON = "sesion";

/** The login screen, telling the family their session ended without them. */
export const SESSION_ENDED_ROUTE = {
  pathname: LOGIN_ROUTE,
  params: { [SIGN_IN_REASON_PARAM]: SESSION_ENDED_REASON },
} as const;

export const SESSION_ENDED_NOTICE = "Tu sesión terminó. Vuelve a entrar.";

import { HOME_ROUTE, LOGIN_ROUTE } from "./routes";

// An in-app path: one leading slash, then anything but a second slash or a
// backslash (both would let "//host" or "/\host" name another origin).
const IN_APP_PATH = /^\/(?![/\\])/;
const SCHEME = /^[a-z][a-z0-9+.-]*:/i;

/**
 * The route to open once the family is signed in: the one they were going to
 * when the login screen stopped them, or Inicio.
 *
 * The value comes from a URL anyone can craft, so only an in-app path is
 * honoured; anything that could name another app or host falls back to Inicio.
 */
export function destinationAfterSignIn(requested: string | string[] | undefined): string {
  const path = Array.isArray(requested) ? requested[0] : requested;
  if (!path || path === LOGIN_ROUTE || SCHEME.test(path) || !IN_APP_PATH.test(path)) {
    return HOME_ROUTE;
  }
  return path;
}

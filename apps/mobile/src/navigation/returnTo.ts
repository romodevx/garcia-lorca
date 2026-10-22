import { HOME_ROUTE, LOGIN_ROUTE } from "./routes";

// An in-app path: one leading slash, then anything but a second slash or a
// backslash (both would let "//host" or "/\host" name another origin).
const IN_APP_PATH = /^\/(?![/\\])/;
const SCHEME = /^[a-z][a-z0-9+.-]*:/i;
const QUERY_OR_FRAGMENT = /[?#]/;

/** The href with percent-escapes resolved, or null when they are malformed. */
function decoded(href: string): string | null {
  try {
    return decodeURIComponent(href);
  } catch {
    return null;
  }
}

/** An href that stays inside the app, judged both as written and decoded. */
function isInAppHref(href: string): boolean {
  const plain = decoded(href);
  if (plain === null) {
    return false;
  }
  return [href, plain].every((form) => !SCHEME.test(form) && IN_APP_PATH.test(form));
}

/** The route part of an href: everything before its query or fragment. */
function routeOf(href: string): string {
  const [route = ""] = href.split(QUERY_OR_FRAGMENT, 1);
  return route;
}

/**
 * The route to open once the family is signed in: the one they were going to
 * when the login screen stopped them (query included), or Inicio.
 *
 * The value comes from a URL anyone can craft, so only an in-app href is
 * honoured — also once its percent-escapes are decoded, so "/%2F/evil" cannot
 * smuggle a "//host" past the check. The login screen itself, with whatever
 * query it carries, is never a destination.
 */
export function destinationAfterSignIn(requested: string | string[] | undefined): string {
  const href = Array.isArray(requested) ? requested[0] : requested;
  if (!href || !isInAppHref(href) || routeOf(href) === LOGIN_ROUTE) {
    return HOME_ROUTE;
  }
  return href;
}

/**
 * Gateway base-URL configuration for the family app. The URL is read from
 * `EXPO_PUBLIC_API_URL` (Expo inlines `EXPO_PUBLIC_*` at build time, mirroring how
 * `apps/web` reads `NEXT_PUBLIC_API_URL`) and defaults to the local gateway so a
 * fresh `expo start` talks to a dev backend without extra setup.
 *
 * Only PUBLIC config lives here — never a secret. The gateway is the security
 * boundary; the client just needs to know where it is.
 */
export const API_BASE_URL = process.env.EXPO_PUBLIC_API_URL ?? "http://localhost:8080";

/** Joins a base URL and an absolute path, collapsing a trailing slash on the base. */
export function joinUrl(base: string, path: string): string {
  return `${base.replace(/\/+$/, "")}${path}`;
}

/** Resolves an API path against the configured gateway base URL. */
export function apiUrl(path: string): string {
  return joinUrl(API_BASE_URL, path);
}

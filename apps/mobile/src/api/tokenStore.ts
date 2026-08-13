import AsyncStorage from "@react-native-async-storage/async-storage";

/**
 * Bearer token storage for the mobile posture. The gateway's `/auth/mobile/*`
 * routes return the token pair IN THE BODY (no cookies — a React Native client has
 * no cookie jar), so the app owns persistence.
 *
 * Two layers:
 *   - an in-memory mirror (`cache`) so `apiFetch` can read the access token
 *     synchronously on the request hot path, without awaiting storage;
 *   - AsyncStorage as the durable backing that survives app relaunches.
 *
 * NOTE(pendiente-hardening): AsyncStorage is unencrypted at rest. For production
 * the pair should move to the OS keychain via `expo-secure-store`; the module
 * boundary here (save/load/clear + the sync getters) is what a secure-store swap
 * would keep, so callers would not change.
 */
export interface TokenPair {
  accessToken: string;
  refreshToken: string;
}

const ACCESS_KEY = "gl.auth.accessToken";
const REFRESH_KEY = "gl.auth.refreshToken";

let cache: TokenPair | null = null;

// Monotonic session-generation counter. Every sign-out (clearTokens) advances it,
// giving in-flight work a way to detect that the session it began under has since
// been cleared. This is the guard that stops a refresh() resolving AFTER a logout
// from resurrecting the just-cleared session (bug_009). Saving tokens does NOT
// advance it: a fresh login inherits the post-logout generation, so a stale
// in-flight refresh still sees a changed generation and drops its result.
let sessionGeneration = 0;

/**
 * The current session generation. Callers sample it before starting async work
 * and re-read it after, dropping the result if it advanced (a sign-out happened
 * in between).
 */
export function currentSessionGeneration(): number {
  return sessionGeneration;
}

/** The current access token, or null when signed out. Synchronous by design. */
export function getAccessToken(): string | null {
  return cache?.accessToken ?? null;
}

/** The current refresh token, or null when signed out. Synchronous by design. */
export function getRefreshToken(): string | null {
  return cache?.refreshToken ?? null;
}

/** Persists a token pair and updates the in-memory mirror. */
export async function saveTokens(pair: TokenPair): Promise<void> {
  cache = pair;
  await AsyncStorage.multiSet([
    [ACCESS_KEY, pair.accessToken],
    [REFRESH_KEY, pair.refreshToken],
  ]);
}

/**
 * Hydrates the in-memory mirror from durable storage (call once on boot). Returns
 * the pair, or null when either token is missing — a partial pair is treated as
 * signed out.
 */
export async function loadTokens(): Promise<TokenPair | null> {
  const entries = await AsyncStorage.multiGet([ACCESS_KEY, REFRESH_KEY]);
  const stored = new Map(entries);
  const accessToken = stored.get(ACCESS_KEY);
  const refreshToken = stored.get(REFRESH_KEY);
  if (!accessToken || !refreshToken) {
    cache = null;
    return null;
  }
  cache = { accessToken, refreshToken };
  return cache;
}

/** Clears the token pair from memory and durable storage (sign out). */
export async function clearTokens(): Promise<void> {
  cache = null;
  // Advance the generation FIRST so any concurrent in-flight refresh that resolves
  // after this point observes the change synchronously, even before the durable
  // multiRemove settles (the multiSet/multiRemove race in bug_009).
  sessionGeneration += 1;
  await AsyncStorage.multiRemove([ACCESS_KEY, REFRESH_KEY]);
}

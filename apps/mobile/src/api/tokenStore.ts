import AsyncStorage from "@react-native-async-storage/async-storage";
import * as SecureStore from "expo-secure-store";

/**
 * Bearer token storage for the mobile posture. The gateway's `/auth/mobile/*`
 * routes return the token pair IN THE BODY (no cookies — a React Native client has
 * no cookie jar), so the app owns persistence.
 *
 * Two layers:
 *   - an in-memory mirror (`cache`) so `apiFetch` can read the access token
 *     synchronously on the request hot path, without awaiting storage;
 *   - SecureStore as the encrypted durable backing that survives app relaunches.
 */
export interface TokenPair {
  accessToken: string;
  refreshToken: string;
}

const ACCESS_KEY = "gl.auth.accessToken";
const REFRESH_KEY = "gl.auth.refreshToken";
const SECURE_TOKEN_KEY = "gl.auth.tokenPair";

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
  await SecureStore.setItemAsync(SECURE_TOKEN_KEY, JSON.stringify(pair));
  await AsyncStorage.multiRemove([ACCESS_KEY, REFRESH_KEY]);
}

/**
 * Hydrates the in-memory mirror from durable storage (call once on boot). Returns
 * the pair, or null when either token is missing — a partial pair is treated as
 * signed out.
 */
export async function loadTokens(): Promise<TokenPair | null> {
  const secureValue = await SecureStore.getItemAsync(SECURE_TOKEN_KEY);
  if (secureValue !== null) {
    try {
      const pair: unknown = JSON.parse(secureValue);
      if (isTokenPair(pair)) {
        cache = pair;
        await AsyncStorage.multiRemove([ACCESS_KEY, REFRESH_KEY]);
        return cache;
      }
    } catch {
      // Invalid secure data is treated as an absent session.
    }
  }

  const entries = await AsyncStorage.multiGet([ACCESS_KEY, REFRESH_KEY]);
  const stored = new Map(entries);
  const accessToken = stored.get(ACCESS_KEY);
  const refreshToken = stored.get(REFRESH_KEY);
  if (!accessToken || !refreshToken) {
    cache = null;
    return null;
  }
  cache = { accessToken, refreshToken };
  await SecureStore.setItemAsync(SECURE_TOKEN_KEY, JSON.stringify(cache));
  await AsyncStorage.multiRemove([ACCESS_KEY, REFRESH_KEY]);
  return cache;
}

/** Clears the token pair from memory and durable storage (sign out). */
export async function clearTokens(): Promise<void> {
  cache = null;
  // Advance the generation FIRST so any concurrent in-flight refresh that resolves
  // after this point observes the change synchronously, even before the durable
  // multiRemove settles (the multiSet/multiRemove race in bug_009).
  sessionGeneration += 1;
  await Promise.all([
    SecureStore.deleteItemAsync(SECURE_TOKEN_KEY),
    AsyncStorage.multiRemove([ACCESS_KEY, REFRESH_KEY]),
  ]);
}

function isTokenPair(value: unknown): value is TokenPair {
  if (typeof value !== "object" || value === null) return false;
  const pair = value as Partial<TokenPair>;
  return typeof pair.accessToken === "string" && typeof pair.refreshToken === "string";
}

import { apiUrl } from "./config";
import { ApiError } from "./errors";
import { emitSessionLost } from "./session";
import {
  clearTokens,
  currentSessionGeneration,
  getRefreshToken,
  saveTokens,
} from "./tokenStore";

/**
 * Mobile auth against the gateway's Bearer posture (`/api/v1/auth/mobile/*`). Both
 * tokens are returned in the JSON body — never a cookie — and stored via
 * `tokenStore`. The access token is then sent as `Authorization: Bearer …` by
 * `apiFetch`.
 */

// Gateway response shapes (mobile posture). See services/gateway auth.go:
//   mobileLoginResponse / mobileRefreshResponse.
interface MobileLoginResponse {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  mustChangePassword: boolean;
}

interface MobileRefreshResponse {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
}

export interface LoginResult {
  /** The gateway asks the user to rotate a temporary password before continuing. */
  mustChangePassword: boolean;
}

const JSON_HEADERS = { "Content-Type": "application/json" } as const;

/**
 * Exchanges credentials for a token pair and stores it. Throws an ApiError on any
 * non-2xx (e.g. 401 for bad credentials) without touching the stored tokens.
 */
export async function login(email: string, password: string): Promise<LoginResult> {
  const res = await fetch(apiUrl("/api/v1/auth/mobile/login"), {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    throw new ApiError(res.status, "login failed");
  }
  const data = (await res.json()) as MobileLoginResponse;
  await saveTokens({ accessToken: data.accessToken, refreshToken: data.refreshToken });
  return { mustChangePassword: Boolean(data.mustChangePassword) };
}

/**
 * Rotates the token pair using the stored refresh token. Returns false (without a
 * network call) when there is no refresh token, and clears the stored pair when
 * the gateway rejects it — a rejected refresh means the session is dead.
 */
export async function refresh(): Promise<boolean> {
  const refreshToken = getRefreshToken();
  if (!refreshToken) {
    return false;
  }
  // Sample the session generation BEFORE the network round-trip so we can detect a
  // logout that lands while the POST is in flight (bug_009).
  const startGeneration = currentSessionGeneration();
  const res = await fetch(apiUrl("/api/v1/auth/mobile/refresh"), {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify({ refreshToken }),
  });
  if (!res.ok) {
    await clearTokens();
    // A rejected refresh token is the one true dead-session moment: signal it so
    // the app root forces navigation back to Login (bug_007). This never fires
    // for the pre-login "no token" case — that path returned false above without
    // reaching the network.
    emitSessionLost();
    return false;
  }
  const data = (await res.json()) as MobileRefreshResponse;
  // A cleared session must never be resurrected by an in-flight refresh: if a
  // logout advanced the generation while we were awaiting the network, drop the
  // freshly-minted pair instead of persisting it over the cleared session.
  if (currentSessionGeneration() !== startGeneration) {
    return false;
  }
  await saveTokens({ accessToken: data.accessToken, refreshToken: data.refreshToken });
  return true;
}

/** Signs out by clearing the stored token pair. */
export async function logout(): Promise<void> {
  await clearTokens();
}

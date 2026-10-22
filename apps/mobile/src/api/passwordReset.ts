import { apiUrl } from "./config";
import { ApiError } from "./errors";

/**
 * Asks the gateway to email a link for a new password
 * (`POST /api/v1/auth/password-reset`, public, rate-limited). The gateway
 * answers 204 whether or not the address has an account, so success says
 * nothing about who is registered; the only other answer is the limiter's 429.
 * Throws an {@link ApiError} on a non-2xx and lets a network failure through.
 */
export async function requestPasswordReset(email: string): Promise<void> {
  const res = await fetch(apiUrl("/api/v1/auth/password-reset"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  });
  if (!res.ok) {
    throw new ApiError(res.status, "password reset failed");
  }
}

import { apiUrl } from "./config";
import { ApiError } from "./errors";
import { saveTokens } from "./tokenStore";

/**
 * Signing in through the external identity provider.
 *
 * The parent types their email and password into the SAME form as before. What
 * changes is who checks them: Identity Platform does, and hands back an ID
 * token, which we exchange at the gateway for OUR token pair.
 *
 * The exchange is the point. Carrying the provider's own token instead would
 * hand session lifetime to the provider and quietly undo our TTLs, our
 * rotation, our revocation, and the session-generation guard this app relies on
 * to keep an in-flight refresh from resurrecting a signed-out session.
 */

/**
 * Obtains an ID token from the provider for these credentials. Injected so this
 * module can be tested without the network, and so the provider stays one import
 * away from being replaced.
 *
 * Rejects when the provider refuses the credentials.
 */
export type IdTokenProvider = (email: string, password: string) => Promise<string>;

interface MobileLoginResponse {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  mustChangePassword: boolean;
}

export interface LoginResult {
  mustChangePassword: boolean;
}

/**
 * Authenticates against the provider and exchanges the result for our session.
 *
 * Returns the same shape as the password login, because it ends in the same
 * place: our pair in `tokenStore`. Nothing about the provider leaks past here.
 */
export async function loginWithProvider(
  email: string,
  password: string,
  getIdToken: IdTokenProvider,
): Promise<LoginResult> {
  // If the provider refuses there is nothing to exchange. Letting the error out
  // here also avoids spending a request against the gateway's shared login rate
  // limit on a call that cannot succeed.
  const idToken = await getIdToken(email, password);

  const res = await fetch(apiUrl("/api/v1/auth/mobile/google"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    // The ID token only. The password went to the provider and has no business
    // reaching our API: sending it would put it in our logs and request history
    // for no gain.
    body: JSON.stringify({ idToken }),
  });
  if (!res.ok) {
    // The provider vouched for them; we did not. A valid provider account with
    // no user row here must not end up signed in, and nothing is stored.
    throw new ApiError(res.status, "provider login failed");
  }
  const data = (await res.json()) as MobileLoginResponse;
  await saveTokens({ accessToken: data.accessToken, refreshToken: data.refreshToken });
  return { mustChangePassword: Boolean(data.mustChangePassword) };
}

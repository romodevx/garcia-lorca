import type { LoginResult } from "./api";

/**
 * Signing in through the external identity provider.
 *
 * The person types their email and password into the SAME form as before. What
 * changes is who checks them: Identity Platform does, and hands back an ID
 * token, which we exchange at the gateway for the HttpOnly cookies this app
 * already runs on.
 *
 * The exchange is the point. The provider's SDK keeps its token in IndexedDB,
 * where any script on the page can read it; our cookies cannot be read by
 * script at all. So the token touches the gateway once and is then forgotten —
 * it never becomes the thing the app carries around.
 */

/**
 * Obtains an ID token from the provider for these credentials. Injected so this
 * module can be tested without the network, and so the provider stays one
 * import away from being replaced.
 *
 * Rejects when the provider refuses the credentials.
 */
export type IdTokenProvider = (email: string, password: string) => Promise<string>;

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/**
 * Authenticates against the provider and exchanges the result for our session.
 *
 * Returns the same shape as the password login, because it ends in the same
 * place: cookies the browser holds and JavaScript cannot read. Nothing secret
 * is returned to the caller.
 */
export async function loginWithProvider(
  baseURL: string,
  email: string,
  password: string,
  getIdToken: IdTokenProvider,
): Promise<LoginResult> {
  // If the provider refuses, there is nothing to exchange. Letting the error
  // out here also avoids spending a request against the gateway's shared login
  // rate limit on a call that cannot succeed.
  const idToken = await getIdToken(email, password);

  const res = await fetch(url(baseURL, "/api/v1/auth/google"), {
    method: "POST",
    // Required: this is what lets the browser STORE the Set-Cookie the gateway
    // replies with. Without it the exchange succeeds and the session is dropped.
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    // The ID token only. The password went to the provider and has no business
    // reaching our API: sending it would put it in our logs and request history
    // for no gain, and keep us in the credential-handling business we are
    // leaving.
    body: JSON.stringify({ idToken }),
  });
  if (!res.ok) {
    // The provider vouched for them; we did not. A valid provider account with
    // no user row here must not end up signed in.
    throw new Error(`login: HTTP ${res.status}`);
  }
  return (await res.json()) as LoginResult;
}

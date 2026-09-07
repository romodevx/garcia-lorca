import { login } from "./auth";
import { providerLoginEnabled } from "./providerFlag";
import type { LoginResult } from "./provider";

/**
 * The one place that decides HOW this app signs somebody in.
 *
 * The parent sees the same form either way; what changes is who checks the
 * password. Keeping the decision here means the login screen never learns that
 * a provider exists, and the day the old path is retired only this file changes.
 */

/** Seams, so the choice can be tested without a provider or a network. */
export interface SignInPaths {
  enabled: boolean;
  viaProvider: (email: string, password: string) => Promise<LoginResult>;
  viaOwnRoute: (email: string, password: string) => Promise<LoginResult>;
}

/**
 * NOTE ON THE LAZY IMPORT: unlike the web, this does NOT keep the SDK out of the
 * bundle — Metro bundles what is reachable, so a build with the flag off still
 * ships it. What it avoids is the initialisation cost on every app start for
 * users who never sign in through the provider. Saying otherwise here would be
 * claiming a saving the build does not make.
 */
async function defaults(): Promise<SignInPaths> {
  const enabled = providerLoginEnabled();
  if (!enabled) {
    return { enabled, viaProvider: async () => { throw new Error("provider login is off"); }, viaOwnRoute: login };
  }
  const [{ loginWithProvider }, { firebaseIdToken }] = await Promise.all([
    import("./provider"),
    import("./firebase"),
  ]);
  return {
    enabled,
    viaProvider: (email, password) => loginWithProvider(email, password, firebaseIdToken),
    viaOwnRoute: login,
  };
}

/**
 * Signs in by whichever path this build is configured for.
 *
 * There is deliberately NO fallback from the provider to the password route: a
 * fallback would be a second door for exactly the credentials the provider just
 * rejected, and would keep the old password store authoritative after the
 * cutover — the migration undone by a catch block.
 */
export async function signIn(email: string, password: string, paths?: SignInPaths): Promise<LoginResult> {
  const { enabled, viaProvider, viaOwnRoute } = paths ?? (await defaults());
  return enabled ? viaProvider(email, password) : viaOwnRoute(email, password);
}

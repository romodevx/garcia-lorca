import { logout } from "./auth";
import { providerLoginEnabled } from "./providerFlag";

/**
 * Ending a session — both halves of it.
 *
 * Clearing our token pair is only half the session once the provider is in
 * play. If a provider session survives, a fresh ID token can be minted with no
 * password and exchanged at `/api/v1/auth/mobile/google` for a complete new
 * pair, so "Cerrar sesión" would log nobody out.
 *
 * On a phone that matters when the phone changes hands or is lost: the person
 * who signed out believes they are out.
 */

/** The provider module, loaded only when it is actually in use. */
export type ProviderModule = { providerSignOut: () => Promise<void> };

export async function endSession(
  clearOurs: () => Promise<void>,
  usingProvider: boolean,
  loadProvider: () => Promise<ProviderModule>,
): Promise<void> {
  // Both are attempted regardless of the other's outcome. A provider that
  // cannot be reached must never strand somebody in a session they asked to
  // leave; and a failure on our side must not leave alive the half that can
  // mint a NEW session.
  const ours = clearOurs().catch(() => {});
  const theirs = usingProvider
    ? loadProvider()
        .then((m) => m.providerSignOut())
        .catch(() => {})
    : Promise.resolve();

  await Promise.all([ours, theirs]);
}

/**
 * The wiring the app uses: our token store, plus the provider when configured.
 *
 * `logout` and the flag are STATIC imports on purpose. They were dynamic at
 * first and a test caught what that costs: if the import fails, the whole
 * sign-out throws and the tokens survive — somebody presses "Cerrar sesión" on
 * a shared family phone and stays signed in. Clearing our own session must not
 * depend on module loading working. Only the provider SDK stays lazy, because
 * only it is heavy and only it is optional; neither of these two pulls it in
 * (that is exactly why providerFlag has no SDK import).
 */
export async function signOut(): Promise<void> {
  await endSession(logout, providerLoginEnabled(), () => import("./firebase"));
}

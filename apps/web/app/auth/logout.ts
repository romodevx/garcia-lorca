/**
 * Ending a session — both halves of it.
 *
 * Our HttpOnly cookies are only half the session once the provider is in play.
 * The SDK can hold a provider session of its own, and while that lives a fresh
 * ID token can be minted with no password and exchanged at
 * `/api/v1/auth/google` for a complete new set of cookies. Clearing only our
 * side would mean "Cerrar sesión" logs nobody out.
 *
 * The scenario that makes this matter is the one the panel is built for: the
 * shared computer at the front desk. Somebody clicks the button and walks away
 * believing they are out. If the provider session survives, the next person to
 * sit down can be them.
 */

/** The provider module, loaded only when it is actually in use. */
export type ProviderModule = { providerSignOut: () => Promise<void> };

export async function endSession(
  baseURL: string,
  apiLogout: (baseURL: string) => Promise<void>,
  usingProvider: boolean,
  loadProvider: () => Promise<ProviderModule>,
): Promise<void> {
  // Both are attempted regardless of the other's outcome. A provider that
  // cannot be reached must never strand somebody in a session they asked to
  // leave; and a failing gateway must not leave alive the half that can mint a
  // NEW session.
  const ours = apiLogout(baseURL).catch(() => {});

  // Not loaded at all when the provider is unused: pulling the SDK to sign out
  // of a session that was never started would cost 468 KB for nothing.
  const theirs = usingProvider
    ? loadProvider()
        .then((m) => m.providerSignOut())
        .catch(() => {})
    : Promise.resolve();

  await Promise.all([ours, theirs]);
}

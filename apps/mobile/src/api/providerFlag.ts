/**
 * Whether this build signs in through the external identity provider.
 *
 * Its own module, with NO import of the provider SDK — the same separation the
 * web app makes, for the same measured reason: reading a flag must not drag the
 * SDK into the bundle. Here the cost lands on a parent's phone and on the
 * download they make over whatever connection they have.
 *
 * An env flag, inlined by Expo at build time, matching how this app already
 * reads its gateway URL. Turning it off is a rebuild, which is the reversibility
 * the door everyone walks through needs — and it is the only kind available
 * anyway, since login happens before there is a session to ask the gateway with.
 */
export function providerLoginEnabled(): boolean {
  return (
    process.env.EXPO_PUBLIC_AUTH_PROVIDER === "identity-platform" &&
    (process.env.EXPO_PUBLIC_FIREBASE_API_KEY ?? "") !== ""
  );
}

/**
 * Whether this build signs in through the external identity provider.
 *
 * Its own module, with NO import of the provider SDK, and that separation is
 * the point: `firebase.ts` is loaded lazily so a build with the flag off never
 * ships it. Reading the flag from the same file as the SDK would drag the SDK
 * into the bundle to answer a question about an environment variable — measured
 * at 468 KB of client JavaScript for a school panel that would never use it.
 *
 * An env flag rather than a runtime one, matching how this project already
 * handles flags: turning it off is a deploy, which is the reversibility the
 * door everyone walks through needs. It is also the only kind available here —
 * runtime flags come from the gateway, and login happens before there is a
 * session to ask with.
 */
export function providerLoginEnabled(): boolean {
  return (
    process.env.NEXT_PUBLIC_AUTH_PROVIDER === "identity-platform" &&
    (process.env.NEXT_PUBLIC_FIREBASE_API_KEY ?? "") !== ""
  );
}

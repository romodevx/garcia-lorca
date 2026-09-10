import { type FirebaseApp, getApps, initializeApp } from "firebase/app";
import { getAuth, inMemoryPersistence, setPersistence, signInWithEmailAndPassword, signOut } from "firebase/auth";

import type { IdTokenProvider } from "./provider";

/**
 * The Identity Platform binding — the ONE file in the web app that knows the
 * provider is Google. Everything else depends on `IdTokenProvider`, a
 * one-function type, so the provider can be replaced or removed without
 * touching the login screen.
 *
 * WHY THE IMPORTS ARE NARROW: `firebase/app` and `firebase/auth` only. The
 * package also ships Firestore, Storage, Functions and Analytics, none of which
 * this product uses.
 */

/**
 * The provider is configured entirely from build-time env. There is nothing
 * secret here — a Firebase web config is public by design, and the security
 * boundary is the gateway plus the provider's own rules, never the config's
 * obscurity.
 */
function config(): { apiKey: string; authDomain: string; projectId: string } {
  return {
    apiKey: process.env.NEXT_PUBLIC_FIREBASE_API_KEY ?? "",
    authDomain: process.env.NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN ?? "",
    projectId: process.env.NEXT_PUBLIC_FIREBASE_PROJECT_ID ?? "",
  };
}

let app: FirebaseApp | undefined;

/** Initialised once, lazily: a login screen nobody has used should cost nothing. */
function firebaseApp(): FirebaseApp {
  app ??= getApps()[0] ?? initializeApp(config());
  return app;
}

/**
 * Signs in at the provider, returns the ID token, and then FORGETS the session.
 *
 * The two lines that look like ceremony are the security of this file, and the
 * first version of it had neither:
 *
 *   inMemoryPersistence — the SDK defaults to browserLocalPersistence, which
 *   writes the provider's REFRESH token to IndexedDB and leaves it there
 *   indefinitely, readable by any script on the origin. That token mints fresh
 *   ID tokens with no password, and `POST /api/v1/auth/google` exchanges those
 *   for a full set of session cookies. It would hand an XSS a renewable key to
 *   the building — the exact attack this exchange exists to prevent, made worse
 *   than before the provider existed, since nothing of the sort used to be
 *   reachable from JavaScript at all.
 *
 *   signOut — even in memory, leaving the session live means the credential
 *   outlives the one request that needed it. After the exchange, the gateway's
 *   HttpOnly cookies are the session; the provider's copy is dead weight with a
 *   blast radius.
 *
 * Net effect: the ID token exists for the duration of one exchange and then
 * cannot be obtained again without the password. Which is what the rest of this
 * code already claimed.
 */
export const firebaseIdToken: IdTokenProvider = async (email, password) => {
  const auth = getAuth(firebaseApp());
  // Before signing in, so no persistent record is ever written.
  await setPersistence(auth, inMemoryPersistence);
  const credential = await signInWithEmailAndPassword(auth, email, password);
  const idToken = await credential.user.getIdToken();
  // Best effort: the exchange already has what it needs, and failing to tidy up
  // must not fail a login that otherwise succeeded.
  await signOut(auth).catch(() => {});
  return idToken;
};

/**
 * Ends any provider session this browser still holds.
 *
 * Called on logout and on idle timeout. With `inMemoryPersistence` above there
 * should be nothing left to end, and it is called anyway: this app has been
 * deployed with the other persistence, browsers hold what they hold, and
 * "should be empty" is not a thing to bet a shared front-desk machine on.
 */
export async function providerSignOut(): Promise<void> {
  if (getApps().length === 0) return; // never initialised; nothing to end.
  await signOut(getAuth(firebaseApp())).catch(() => {});
}

import { type FirebaseApp, getApps, initializeApp } from "firebase/app";
import { getAuth, inMemoryPersistence, initializeAuth, signInWithEmailAndPassword, signOut } from "firebase/auth";

import type { IdTokenProvider } from "./provider";

/**
 * The Identity Platform binding — the ONE file in the family app that knows the
 * provider is Google. Everything else depends on `IdTokenProvider`, a
 * one-function type, so the provider can be replaced or removed without
 * touching the login screen.
 *
 * Narrow imports: `firebase/app` and `firebase/auth` only. The package also
 * ships Firestore, Storage, Functions and Analytics, none of which this app
 * uses — and here the weight lands on a parent's phone and on their data plan.
 */

function config(): { apiKey: string; authDomain: string; projectId: string } {
  return {
    apiKey: process.env.EXPO_PUBLIC_FIREBASE_API_KEY ?? "",
    authDomain: process.env.EXPO_PUBLIC_FIREBASE_AUTH_DOMAIN ?? "",
    projectId: process.env.EXPO_PUBLIC_FIREBASE_PROJECT_ID ?? "",
  };
}

let app: FirebaseApp | undefined;

function firebaseApp(): FirebaseApp {
  app ??= getApps()[0] ?? initializeApp(config());
  return app;
}

/**
 * IN-MEMORY PERSISTENCE, DECLARED AT INITIALISATION, and this is the security of
 * the file rather than ceremony.
 *
 * On React Native the SDK's default is to persist the session — the provider's
 * REFRESH token written to storage and left there. That token mints fresh ID
 * tokens with no password, and `/api/v1/auth/mobile/google` exchanges those for
 * a full session. It would put a renewable key to the building in the app's
 * unencrypted storage, next to the pair we already flag as needing the OS
 * keychain, and for no benefit at all: after the exchange OUR pair is the
 * session, and the provider's copy is dead weight with a blast radius.
 *
 * `initializeAuth` rather than `getAuth` because persistence must be settled
 * BEFORE the first sign-in — on the web the equivalent is setPersistence, but
 * here choosing at initialisation is the only way to never have written.
 */
function auth(): ReturnType<typeof getAuth> {
  const instance = firebaseApp();
  try {
    return initializeAuth(instance, { persistence: inMemoryPersistence });
  } catch {
    // Already initialised (Fast Refresh, or a second sign-in attempt): reuse it.
    // The persistence choice above is what that instance was built with.
    return getAuth(instance);
  }
}

/**
 * Signs in at the provider, returns the ID token, and then FORGETS the session.
 * The token exists for the duration of one exchange and cannot be obtained
 * again without the password.
 */
export const firebaseIdToken: IdTokenProvider = async (email, password) => {
  const instance = auth();
  const credential = await signInWithEmailAndPassword(instance, email, password);
  const idToken = await credential.user.getIdToken();
  // Best effort: the exchange already has what it needs, and failing to tidy up
  // must not fail a login that otherwise succeeded.
  await signOut(instance).catch(() => {});
  return idToken;
};

/** Ends any provider session this app still holds. Called on sign-out. */
export async function providerSignOut(): Promise<void> {
  if (getApps().length === 0) return; // never initialised; nothing to end.
  await signOut(auth()).catch(() => {});
}

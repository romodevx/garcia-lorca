import { type FirebaseApp, getApps, initializeApp } from "firebase/app";
import { getAuth, signInWithEmailAndPassword } from "firebase/auth";

import type { IdTokenProvider } from "./provider";

/**
 * The Identity Platform binding — the ONE file in the web app that knows the
 * provider is Google.
 *
 * Everything else depends on `IdTokenProvider`, a one-function type, so the
 * provider can be replaced or removed without touching the login screen.
 *
 * WHY THE IMPORTS ARE NARROW: `firebase/app` and `firebase/auth` only. The
 * package also ships Firestore, Storage, Functions and Analytics, none of which
 * this product uses; importing the umbrella would put all of them in the bundle
 * of a school panel that runs on whatever laptop the office has.
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
 * Signs in at the provider and returns the resulting ID token.
 *
 * The token goes straight to the gateway and is not stored by us. The SDK does
 * keep its own copy in IndexedDB — that is why the exchange exists, so the app
 * itself never has to carry it.
 */
export const firebaseIdToken: IdTokenProvider = async (email, password) => {
  const credential = await signInWithEmailAndPassword(getAuth(firebaseApp()), email, password);
  return credential.user.getIdToken();
};

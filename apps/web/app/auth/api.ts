/**
 * Thin fetch helpers for the gateway auth endpoints. Access + refresh tokens live
 * ONLY in HttpOnly cookies the gateway sets — they are never in the JSON body and
 * are unreadable by JS. Every call uses credentials:"include" so the browser
 * carries those cookies; the responses expose only non-secret metadata.
 */
export interface LoginResult {
  expiresIn: number;
  mustChangePassword: boolean;
}

export interface RefreshResult {
  expiresIn: number;
}

export interface MeResult {
  userId: string;
  email: string;
  permissions: string[];
  // The gateway enriches /me from iam when available: the real display name and
  // the caller's primary role name. Both are optional — a directory outage
  // degrades /me to the token principal, so the chrome must tolerate their
  // absence (it falls back to a name derived from the email).
  displayName?: string;
  role?: string;
}

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

export async function login(baseURL: string, email: string, password: string): Promise<LoginResult> {
  const res = await fetch(url(baseURL, "/api/v1/auth/login"), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    throw new Error(`login: HTTP ${res.status}`);
  }
  return (await res.json()) as LoginResult;
}

// me restores the session on boot: it reads the authenticated principal using
// the HttpOnly cookie the browser still holds. A 200 means a live session whose
// forced password change (if any) is already cleared, so the caller can go
// straight to the dashboard; any non-200 (401 no session, 403 change pending)
// throws so the caller shows the login screen.
export async function me(baseURL: string): Promise<MeResult> {
  const res = await fetch(url(baseURL, "/api/v1/me"), {
    method: "GET",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(`me: HTTP ${res.status}`);
  }
  return (await res.json()) as MeResult;
}

// refresh rotates both auth cookies. It sends no body — the gateway reads the
// refresh token from the gl_refresh cookie (carried by credentials:"include").
export async function refresh(baseURL: string): Promise<RefreshResult> {
  const res = await fetch(url(baseURL, "/api/v1/auth/refresh"), {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(`refresh: HTTP ${res.status}`);
  }
  return (await res.json()) as RefreshResult;
}

export async function changePassword(baseURL: string, currentPassword: string, newPassword: string): Promise<void> {
  const res = await fetch(url(baseURL, "/api/v1/auth/change-password"), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ currentPassword, newPassword }),
  });
  if (!res.ok) {
    throw new Error(`changePassword: HTTP ${res.status}`);
  }
}

export async function logout(baseURL: string): Promise<void> {
  const res = await fetch(url(baseURL, "/api/v1/auth/logout"), {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(`logout: HTTP ${res.status}`);
  }
}

/**
 * Redeem an invitation and set the password its owner just chose.
 *
 * The token goes in the BODY, never the URL. The activation page reads it from
 * its own query string — where the mail had to put it — and posts it from
 * there, so it does not travel again into access logs or into the Referer
 * header the page sends to every third-party resource it loads.
 *
 * A dead token throws. Unknown, expired and already-used are one error on
 * purpose: telling them apart would be a way to probe which tokens exist.
 */
export async function acceptInvitation(baseURL: string, token: string, password: string): Promise<void> {
  const res = await fetch(url(baseURL, "/api/v1/auth/invitations/accept"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token, password }),
  });
  if (!res.ok) {
    throw new Error(`accept invitation: HTTP ${res.status}`);
  }
}

/**
 * Ask for a password-reset link.
 *
 * It NEVER throws for a business reason and never reports whether the address
 * has an account. The screen says the same sentence either way. Anything else
 * would turn this into a way to read the school's staff and guardian directory
 * one request at a time.
 */
export async function requestPasswordReset(baseURL: string, email: string): Promise<void> {
  await fetch(url(baseURL, "/api/v1/auth/password-reset"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  }).catch(() => {
    // Even a network failure is not reported differently: the caller must not be
    // able to tell one outcome from another.
  });
}

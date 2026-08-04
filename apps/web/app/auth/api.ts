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

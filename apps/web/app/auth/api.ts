/**
 * Thin fetch helpers for the gateway auth endpoints. Credentials ("include")
 * carry the HttpOnly gl_access cookie the gateway sets on login; the returned
 * access token is also available for callers that prefer a bearer header.
 */
export interface LoginResult {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  mustChangePassword: boolean;
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

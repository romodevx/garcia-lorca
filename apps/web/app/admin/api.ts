/**
 * Typed fetch helpers for the gateway's iam admin endpoints (`/api/v1/users`,
 * `/api/v1/roles`) plus the gateway-orchestrated teacher↔user link
 * (`/api/v1/config/teachers/{id}/user`), gated behind the `module_iam` flag +
 * cookie auth. The session lives ONLY in the HttpOnly cookie the gateway set, so
 * every call uses `credentials:"include"`; nothing secret ever travels in the
 * body or response.
 *
 * Reads require `users:read`/`roles:read`, writes `users:write`/`roles:write`
 * (the owner role holds all four); the teacher-link route sits under
 * `config:write`. These helpers mirror `app/config/api.ts`: a `url()`
 * trailing-slash guard, throw on any non-2xx, and parse the gateway's
 * `{users:[…]}`/`{roles:[…]}` envelopes. Shapes match the gateway REST contract
 * (services/gateway/internal/handler/admin.go) exactly.
 *
 * SECURITY: this client is UX only. The gateway (behind the flag + the
 * permission guards) is the security boundary; temp passwords are sent once at
 * creation and never returned or logged.
 */

import type { Teacher } from "../teachers/api";

// --- entity types (match the gateway JSON contract) -----------------------

import { authFetch } from "../auth/authFetch";

export type UserStatus = "active" | "suspended";

export interface AdminUser {
  id: string;
  email: string;
  displayName: string;
  status: UserStatus;
  mustChangePassword: boolean;
  roleIds: string[];
}

export interface CreateUserInput {
  email: string;
  displayName: string;
  /** Admin-provided one-time password; the user rotates it at first login. */
  tempPassword: string;
  roleIds: string[];
}

/**
 * Inviting somebody. NO password field, and that absence is the feature: the
 * account is created without one and its owner chooses the first through a link
 * the identity provider emails them. Nobody else ever knows it, and a mistyped
 * address never becomes a working account — the mail simply never arrives.
 */
export interface InviteUserInput {
  email: string;
  displayName: string;
  roleIds: string[];
}

export interface Role {
  id: string;
  name: string;
  permissions: string[];
}

export interface CreateRoleInput {
  name: string;
  permissions: string[];
}

// --- transport helpers ----------------------------------------------------

const BASE_PATH = "/api/v1";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/** POST a JSON body and return the parsed entity. Throws on any non-2xx. */
async function postJSON<T>(baseURL: string, path: string, body: unknown): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`admin ${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(baseURL: string, path: string, key: string): Promise<T[]> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
  if (!res.ok) {
    throw new Error(`admin ${path}: HTTP ${res.status}`);
  }
  const data = (await res.json()) as Record<string, T[] | undefined>;
  return data[key] ?? [];
}

// --- users ----------------------------------------------------------------

export function listUsers(baseURL: string): Promise<AdminUser[]> {
  return getList<AdminUser>(baseURL, "/users", "users");
}

export function createUser(baseURL: string, input: CreateUserInput): Promise<AdminUser> {
  return postJSON<AdminUser>(baseURL, "/users", input);
}

/** Invite: create the account with no password and have the provider mail its
 * owner the link that sets one. Calling it again for the same address resends
 * that link, which is also the administrator password reset. */
export function inviteUser(baseURL: string, input: InviteUserInput): Promise<AdminUser> {
  return postJSON<AdminUser>(baseURL, "/users/invite", input);
}

export function setUserStatus(baseURL: string, id: string, status: UserStatus): Promise<AdminUser> {
  return postJSON<AdminUser>(baseURL, `/users/${id}/status`, { status });
}

export function setUserRoles(baseURL: string, id: string, roleIds: string[]): Promise<AdminUser> {
  return postJSON<AdminUser>(baseURL, `/users/${id}/roles`, { roleIds });
}

// --- roles ----------------------------------------------------------------

export function listRoles(baseURL: string): Promise<Role[]> {
  return getList<Role>(baseURL, "/roles", "roles");
}

export function createRole(baseURL: string, input: CreateRoleInput): Promise<Role> {
  return postJSON<Role>(baseURL, "/roles", input);
}

export function setRolePermissions(baseURL: string, id: string, permissions: string[]): Promise<Role> {
  return postJSON<Role>(baseURL, `/roles/${id}/permissions`, { permissions });
}

// --- teacher ↔ user link --------------------------------------------------

/**
 * Links (or, with an empty `userId`, unlinks) a config teacher to an iam user.
 * The gateway validates the user exists via iam before storing the id on the
 * teacher record.
 */
export function linkTeacherUser(baseURL: string, teacherId: string, userId: string): Promise<Teacher> {
  return postJSON<Teacher>(baseURL, `/config/teachers/${teacherId}/user`, { userId });
}

/**
 * Typed fetch helpers for the gateway's iam admin endpoints (`/api/v1/users`,
 * `/api/v1/roles`) plus the gateway-orchestrated teacher↔user link
 * (`/api/v1/teachers/{id}/user`, behind `module_teachers` + `teachers:write`),
 * the admin routes gated behind the `module_iam` flag +
 * cookie auth. The session lives ONLY in the HttpOnly cookie the gateway set, so
 * every call uses `credentials:"include"`; nothing secret ever travels in the
 * body or response.
 *
 * Reads require `users:read`/`roles:read`, writes `users:write`/`roles:write`
 * (the owner role holds all four); the teacher-link route sits under
 * `teachers:write`. These helpers mirror `app/config/api.ts`: a `url()`
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

/**
 * "invited" is the state that did not exist, and its absence was three bugs.
 *
 * An account was created `active`, so nothing recorded the difference between
 * "somebody made this account" and "somebody is using it" — which is exactly
 * what the resend button needs to know when to hide, and what made a mistyped
 * address into a working account nobody could reach.
 *
 * "unknown" is the gateway's honest answer for a status this build does not
 * recognise. It exists because the gateway used to answer "" and the UI drew
 * those users as suspended: a plausible screen telling a lie.
 */
export type UserStatus = "active" | "suspended" | "invited" | "unknown";

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
 * owner the link that sets one. An address that already has an account is a
 * conflict, not a resend — resendInvitation is that. */
/**
 * What came of issuing a link.
 *
 * `sent` is the field whose absence was the original bug: the identity provider
 * answered 200 to messages it silently dropped, so "invitación enviada" was a
 * sentence nobody could check — not the director, not us. Now the answer says
 * whether the message actually left, and when it did not, why, plus the link so
 * access can still be handed over by hand.
 */
export interface Invitation {
  url: string;
  /** RFC3339. Empty when no invitation was issued. */
  expiresAt: string;
  sent: boolean;
  sendError?: string;
  /**
   * Which road the message took. The two are not equivalent and the director
   * has to know which one they just sent.
   *
   *   "smtp"     our own message: this link, 24 h, our page, in Spanish.
   *   "provider" Firebase's OWN link instead: 1 h, its page in English, and a
   *              shared sender Gmail tends to file as spam. Temporary, until
   *              the school hands over an SMTP credential.
   *   undefined  nothing went out; the link is there to hand over.
   */
  via?: "smtp" | "provider";
}

export interface InvitedUser extends AdminUser {
  invitation: Invitation;
}

export function inviteUser(baseURL: string, input: InviteUserInput): Promise<InvitedUser> {
  return postJSON<InvitedUser>(baseURL, "/users/invite", input);
}

/**
 * Mint a fresh link for somebody who has not walked through the door yet.
 *
 * The backend refuses on an account that is already active. The button being
 * hidden is a courtesy; that refusal is the rule.
 *
 * Returns the outcome rather than nothing. It used to answer 204, which made a
 * delivered invitation and a dropped one look identical from here — which is
 * how the original failure went a week without anybody noticing.
 */
export async function resendInvitation(baseURL: string, id: string): Promise<Invitation> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/users/${id}/invite`), {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) {
    throw new Error(`admin resend invitation: HTTP ${res.status}`);
  }
  return (await res.json()) as Invitation;
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
  return postJSON<Teacher>(baseURL, `/teachers/${teacherId}/user`, { userId });
}

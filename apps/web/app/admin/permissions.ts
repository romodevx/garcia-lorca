const USERS_DELETE = "users:delete";

/**
 * Whether the principal may delete user accounts (T2, owner-only). Unknown
 * permissions offer nothing. UX only — the gateway's 403 and iam's own check
 * are the boundary.
 */
export function canDeleteUsers(permissions: readonly string[] | undefined): boolean {
  return permissions !== undefined && permissions.includes(USERS_DELETE);
}

/**
 * The seeded `guardian` role (migrations/iam/0010). It is given ONLY from
 * "Dar acceso a la app" on the pupil's record, which links the account to the
 * child; iam refuses it on every manual road (ISSUE-CC-115).
 */
export const GUARDIAN_ROLE_ID = "00000000-0000-0000-0000-0000000000a2";

/**
 * The roles an administrator may hand out by hand: every role but guardian.
 * UX only — iam is the boundary.
 */
export function assignableByHand<R extends { readonly id: string }>(roles: readonly R[]): R[] {
  return roles.filter((r) => r.id.toLowerCase() !== GUARDIAN_ROLE_ID);
}

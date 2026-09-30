const USERS_DELETE = "users:delete";

/**
 * Whether the principal may delete user accounts (T2, owner-only). Unknown
 * permissions offer nothing. UX only — the gateway's 403 and iam's own check
 * are the boundary.
 */
export function canDeleteUsers(permissions: readonly string[] | undefined): boolean {
  return permissions !== undefined && permissions.includes(USERS_DELETE);
}

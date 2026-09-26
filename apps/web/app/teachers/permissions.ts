const TEACHERS_WRITE = "teachers:write";

/**
 * Whether the principal may change a teacher's carga académica. Unknown
 * permissions (still resolving) hide nothing: this check is UX only, the
 * backend's answer is the boundary.
 */
export function canEditTeachingLoad(permissions: readonly string[] | undefined): boolean {
  return permissions === undefined || permissions.includes(TEACHERS_WRITE);
}

const USERS_WRITE = "users:write";

/**
 * Whether the principal may give a teacher a login: it links the roster row
 * (teachers:write) AND creates an account (users:write). Unlike the load, an
 * unknown permission set offers nothing — the button creates accounts, so it
 * waits until the answer is in. UX only; the backend's 403 is the boundary.
 */
export function canGrantTeacherAccess(permissions: readonly string[] | undefined): boolean {
  return permissions !== undefined && permissions.includes(TEACHERS_WRITE) && permissions.includes(USERS_WRITE);
}

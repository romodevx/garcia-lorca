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
 * What the maestro role carries (migrations/iam/0022). The gateway's
 * caller-subset guard refuses to grant a role whose permissions the caller
 * does not itself hold, so without all four the button could only ever 403.
 */
const MAESTRO_PERMISSIONS = ["teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"] as const;

/**
 * Whether the principal may give a teacher a login: it links the roster row
 * (teachers:write), creates an account (users:write), and grants the maestro
 * role (all of its permissions). Unlike the load, an unknown permission set
 * offers nothing — the button creates accounts, so it waits until the answer is
 * in. UX only; the backend's 403 is the boundary.
 */
export function canGrantTeacherAccess(permissions: readonly string[] | undefined): boolean {
  if (permissions === undefined) return false;
  const required = [TEACHERS_WRITE, USERS_WRITE, ...MAESTRO_PERMISSIONS];
  return required.every((p) => permissions.includes(p));
}

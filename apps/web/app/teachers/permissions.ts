const TEACHERS_WRITE = "teachers:write";

/**
 * Whether the principal may change a teacher's carga académica. Unknown
 * permissions (still resolving) hide nothing: this check is UX only, the
 * backend's answer is the boundary.
 */
export function canEditTeachingLoad(permissions: readonly string[] | undefined): boolean {
  return permissions === undefined || permissions.includes(TEACHERS_WRITE);
}

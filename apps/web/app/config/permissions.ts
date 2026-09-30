const LEVELS_DELETE = "levels:delete";

/**
 * Whether the principal may delete a level (owner-only). Unknown permissions
 * offer nothing: this is the most destructive action in Configuración, so the
 * button waits for the answer. UX only — the gateway's 403 is the boundary.
 */
export function canDeleteLevels(permissions: readonly string[] | undefined): boolean {
  return permissions !== undefined && permissions.includes(LEVELS_DELETE);
}

const LEVELS_DELETE = "levels:delete";

/**
 * Whether the principal may delete a level (owner-only). Unknown permissions
 * offer nothing: this is the most destructive action in Configuración, so the
 * button waits for the answer. UX only — the gateway's 403 is the boundary.
 */
export function canDeleteLevels(permissions: readonly string[] | undefined): boolean {
  return permissions !== undefined && permissions.includes(LEVELS_DELETE);
}

const CONFIG_WRITE = "config:write";

/**
 * Whether the principal may edit the academic catalog (evaluation periods
 * included). Unknown permissions edit nothing. UX only — the gateway's 403 is
 * the boundary.
 */
export function canWriteConfig(permissions: readonly string[] | undefined): boolean {
  return permissions !== undefined && permissions.includes(CONFIG_WRITE);
}

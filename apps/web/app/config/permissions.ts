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

const FINANCE_READ = "finance:read";

/**
 * Whether the principal may read the tuition plans (GET /finance/tuition-plans
 * answers to finance:read, owner-only). Without it Configuración must not even
 * ask (CC-311): a 403 in the console and an empty Colegiaturas card read as a
 * broken screen. UX only — the gateway's 403 is the boundary.
 */
export function canReadTuition(permissions: readonly string[] | undefined): boolean {
  return permissions !== undefined && permissions.includes(FINANCE_READ);
}

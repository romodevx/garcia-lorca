import type { UserProfile } from "../_design/data";

/** What AppShell hands the home, whichever of the two the flag picks. */
export interface HomeProps {
  user: UserProfile;
  /** Opens a module, optionally on one of its intents (routing/pendingIntent.ts). */
  onOpen: (id: string, intent?: string) => void;
  /**
   * The authenticated principal's READ permissions. Drives which module tiles
   * and KPI shortcuts are visible. `undefined` until `me()` resolves — the grid
   * shows a skeleton rather than flashing every module. UX-only; the gateway is
   * the security boundary.
   */
  permissions?: readonly string[];
  /**
   * The permissions could not be loaded at all (the /me call failed).
   *
   * Distinct from `permissions === undefined`, and the distinction is the whole
   * point: undefined alone rendered the loading skeleton, so a failed call
   * looked exactly like a slow one and shimmered forever. The owner read that
   * as "no hay módulos" and reported it as such (dev, 2026-09-07).
   */
  permissionsFailed?: boolean;
  /** Retry loading them, so the only way out is not signing out again. */
  onRetryPermissions?: () => void;
  /** Injectable clock so the greeting/date are deterministic under test. */
  now?: Date;
}

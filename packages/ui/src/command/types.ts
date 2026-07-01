/**
 * Command Palette — platform-agnostic types.
 *
 * This module has NO React and NO DOM imports so the matching core is shared
 * verbatim by apps/web (cmdk) and apps/mobile (native modal). The palette is the
 * "todo a un buscador, mínimo de clics" surface: it NAVIGATES (never mutates) —
 * every command carries a `route`, and executing a command means going there
 * (e.g. a prefilled form), not performing a side effect.
 *
 * Strings here are ALREADY RESOLVED (title/aliases), not i18n keys: the app
 * resolves `@garcia-lorca/i18n` keys before handing commands to the core, so the
 * core stays pure and locale-agnostic.
 */

/**
 * Result category, in the plan's priority order (lower = shown first):
 *   module → create → recent → record → quick.
 * `record` (Layer B) is included in the type now but only populated from CP-2.
 */
export type CommandKind = "module" | "create" | "recent" | "record" | "quick";

/** Priority weight per kind (lower sorts first). Single source for ranking. */
export const KIND_ORDER: Record<CommandKind, number> = {
  module: 0,
  create: 1,
  recent: 2,
  record: 3,
  quick: 4,
};

export interface Command {
  /** Stable id, e.g. "students.nav" | "students.create" | "record:GL-2041". */
  readonly id: string;
  readonly kind: CommandKind;
  /** Localized, user-visible title (already resolved from i18n). */
  readonly title: string;
  /** Optional localized subtitle (group, guardian, module desc…). */
  readonly subtitle?: string;
  /** Extra localized match terms (a small fixed alias set; NOT synonym lists). */
  readonly aliases?: readonly string[];
  /** Design-system icon name (dashboard-consistent). */
  readonly icon: string;
  /** Design-system color key (keys the group/badge to its module). */
  readonly color?: string;
  /** Destination route. Navigating here IS executing the command. */
  readonly route: string;
  /** Owning module id (for grouping/recents). */
  readonly moduleId?: string;
}

export interface ScoredCommand {
  readonly command: Command;
  /** Higher = better textual match (0 excluded). */
  readonly score: number;
}

export interface MatchOptions {
  /** Max results returned (default 8). */
  readonly limit?: number;
}

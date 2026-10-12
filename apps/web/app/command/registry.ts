import type { Command, CommandKind } from "@garcia-lorca/ui/command-core";
import type { FlagSet } from "@garcia-lorca/ui/flags";

import { MODULES, moduleHasPerm, type ModuleDef } from "../_design/data";
import type { IntentModule, IntentOf } from "../routing/pendingIntent";

/**
 * Layer A command registry — the static half of the palette (nav + create +
 * quick), derived from the module list and localized. Records (Layer B) are
 * added from CP-2 (students first), fed by an owned search index.
 *
 *   MODULES ─┬─ nav command (every enabled module)
 *            └─ ACTIONS ── create/quick commands (enabled modules only)
 *
 * SECURITY: a command is emitted ONLY if its owning module's flag is enabled.
 * A disabled module contributes ZERO commands, so its routes/actions never
 * appear. In this demo SPA the flags come from the client `DEMO_FLAGS`; when
 * auth + gateway land this function runs SERVER-SIDE (RSC) against real per-user
 * flags/role, so disallowed commands never enter the client bundle.
 *
 * Commands NAVIGATE (route `module:<id>`), never mutate — executing means going
 * to the module/prefilled form, matching the "mínimo de clics" north star.
 */

/** Resolves a palette-namespace i18n key to a localized string. */
export type Translate = (key: string) => string;

interface ActionBase {
  readonly kind: Extract<CommandKind, "create" | "quick">;
  readonly titleKey: string;
  readonly aliasKey?: string;
  /** Disambiguator when a module registers several actions of the same kind. */
  readonly key?: string;
  /**
   * Permissions the action itself needs on top of the module's READ (a write
   * shortcut is noise — and a promise the gateway breaks — for a read-only role).
   */
  readonly requires?: readonly string[];
}

/**
 * Optionally with a sub-intent appended to the route (`module:expenses:new`)
 * so the surface opens ON that flow. Typed per module: an intent the
 * destination screen does not understand does not compile.
 */
type ActionSpec = ActionBase &
  ({ readonly moduleId: string; readonly intent?: undefined } | { [M in IntentModule]: { readonly moduleId: M; readonly intent: IntentOf[M] } }[IntentModule]);

/** Curated create/quick actions (seed; each module extends this as it's built). */
export const ACTIONS: readonly ActionSpec[] = [
  { moduleId: "students", kind: "create", titleKey: "create.student", aliasKey: "alias.student", intent: "new", requires: ["students:create"] },
  { moduleId: "students", kind: "quick", titleKey: "quick.searchStudent", aliasKey: "alias.searchStudent", intent: "search" },
  // Enrollment surface (Phase 4) — fewest-clicks shortcuts into the students surface.
  { moduleId: "students", kind: "create", key: "enroll", titleKey: "create.enroll", aliasKey: "alias.enroll" },
  { moduleId: "students", kind: "create", key: "transfer", titleKey: "create.transfer", aliasKey: "alias.transfer" },
  { moduleId: "teachers", kind: "create", titleKey: "create.teacherRoster", aliasKey: "alias.teacherRoster", intent: "new", requires: ["teachers:write"] },
  // The ventanilla lives in Registro de pagos; its search takes the focus on its own.
  { moduleId: "payments", kind: "create", titleKey: "create.payment", aliasKey: "alias.payment" },
  { moduleId: "finance", kind: "quick", titleKey: "quick.overdue", aliasKey: "alias.overdue", intent: "overdue" },
  { moduleId: "expenses", kind: "create", titleKey: "create.expense", aliasKey: "alias.expense", intent: "new", requires: ["expenses:write"] },
  { moduleId: "schedule", kind: "create", titleKey: "create.absence", aliasKey: "alias.absence", intent: "absence", requires: ["schedule:write"] },
  { moduleId: "activities", kind: "create", titleKey: "create.activity", aliasKey: "alias.activity", intent: "new", requires: ["activities:write"] },
  { moduleId: "grades", kind: "create", titleKey: "create.evaluation", aliasKey: "alias.evaluation" },
  // Grade-capture surface (Phase 5) — fewest-clicks shortcut into the roster grid.
  { moduleId: "grades", kind: "create", key: "capture", titleKey: "create.captureGrades", aliasKey: "alias.captureGrades" },
  { moduleId: "announcements", kind: "create", titleKey: "create.announcement", aliasKey: "alias.announcement" },
  { moduleId: "scholarships", kind: "create", titleKey: "create.scholarship", aliasKey: "alias.scholarship", intent: "new", requires: ["scholarships:write"] },
  { moduleId: "documents", kind: "create", titleKey: "create.document", aliasKey: "alias.document" },
  { moduleId: "uniforms", kind: "create", titleKey: "create.uniform", aliasKey: "alias.uniform" },
  { moduleId: "uniforms", kind: "quick", titleKey: "quick.inventory", aliasKey: "alias.inventory", intent: "inventory" },
  { moduleId: "billing", kind: "create", titleKey: "create.invoice", aliasKey: "alias.invoice" },
  // Academic-catalog (module_config) surfaced under the "settings" module.
  { moduleId: "settings", kind: "create", key: "cycle", titleKey: "create.cycle", aliasKey: "alias.cycle" },
  { moduleId: "settings", kind: "create", key: "level", titleKey: "create.level", aliasKey: "alias.level" },
  { moduleId: "settings", kind: "create", key: "group", titleKey: "create.group", aliasKey: "alias.group" },
  { moduleId: "settings", kind: "create", key: "teacher", titleKey: "create.teacher", aliasKey: "alias.teacher" },
  { moduleId: "settings", kind: "create", key: "assignment", titleKey: "create.assignment", aliasKey: "alias.assignment" },
  // Admin surface (module_iam) — fewest-clicks shortcuts into the admin surface.
  { moduleId: "admin", kind: "create", key: "user", titleKey: "create.user", aliasKey: "alias.user" },
  { moduleId: "admin", kind: "create", key: "role", titleKey: "create.role", aliasKey: "alias.role" },
  { moduleId: "attendance", kind: "quick", titleKey: "quick.attendance", aliasKey: "alias.attendance" },
  { moduleId: "reports", kind: "quick", titleKey: "quick.reports", aliasKey: "alias.reports" },
];

const moduleById = new Map<string, ModuleDef>(MODULES.map((m) => [m.id, m]));

/**
 * Whether the caller holds every permission the action requires (no set given ⇒
 * flag-only demo). Shared with the home's shortcuts so the two never drift.
 */
export function holdsAll(required: readonly string[] | undefined, permissions: readonly string[] | undefined): boolean {
  if (required === undefined || permissions === undefined) return true;
  return required.every((p) => permissions.includes(p));
}

/** Split a comma-joined localized alias string into trimmed terms. */
function parseAliases(value: string): string[] {
  return value
    .split(",")
    .map((term) => term.trim())
    .filter((term) => term.length > 0);
}

/**
 * @param permissions the caller's READ permissions. When supplied, a module also
 *   needs its `perm` present here to contribute commands (role-aware visibility,
 *   layered on top of the flag gate). Omit it (undefined) for the flag-only
 *   behavior used by the design demo and flag-isolation unit tests. UX-only —
 *   the gateway 403/404 remains the security boundary.
 */
export function buildCommandRegistry(flags: FlagSet, t: Translate, permissions?: readonly string[]): Command[] {
  const commands: Command[] = [];
  // A module is allowed when its flag is ON and — if a permission set was given —
  // the caller holds its READ permission. Deactivated (hidden) modules never pass.
  const allowed = (module: ModuleDef): boolean =>
    module.hidden !== true &&
    flags[module.flag] === true &&
    (permissions === undefined || moduleHasPerm(module, permissions));

  for (const module of MODULES) {
    if (!allowed(module)) continue;
    commands.push({
      id: `${module.id}.nav`,
      kind: "module",
      title: t(`nav.${module.id}`),
      subtitle: module.desc,
      icon: module.icon,
      color: module.color,
      route: `module:${module.id}`,
      moduleId: module.id,
    });
  }

  for (const action of ACTIONS) {
    const module = moduleById.get(action.moduleId);
    if (module === undefined || !allowed(module) || !holdsAll(action.requires, permissions)) continue;
    commands.push({
      id: action.key !== undefined ? `${module.id}.${action.kind}.${action.key}` : `${module.id}.${action.kind}`,
      kind: action.kind,
      title: t(action.titleKey),
      aliases: action.aliasKey !== undefined ? parseAliases(t(action.aliasKey)) : undefined,
      icon: module.icon,
      color: module.color,
      route: action.intent !== undefined ? `module:${module.id}:${action.intent}` : `module:${module.id}`,
      moduleId: module.id,
    });
  }

  return commands;
}

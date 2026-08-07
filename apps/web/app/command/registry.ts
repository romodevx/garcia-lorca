import type { Command, CommandKind } from "@garcia-lorca/ui/command-core";
import type { FlagSet } from "@garcia-lorca/ui/flags";

import { MODULES, type ModuleDef } from "../_design/data";

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

interface ActionSpec {
  readonly moduleId: string;
  readonly kind: Extract<CommandKind, "create" | "quick">;
  readonly titleKey: string;
  readonly aliasKey?: string;
  /** Disambiguator when a module registers several actions of the same kind. */
  readonly key?: string;
  /**
   * Sub-intent appended to the module route (`module:finance:payment`) so the
   * surface opens directly on that flow. Omitted ⇒ the module's default view.
   */
  readonly intent?: string;
}

/** Curated create/quick actions (seed; each module extends this as it's built). */
const ACTIONS: readonly ActionSpec[] = [
  { moduleId: "students", kind: "create", titleKey: "create.student", aliasKey: "alias.student" },
  // Enrollment surface (Phase 4) — fewest-clicks shortcuts into the students surface.
  { moduleId: "students", kind: "create", key: "enroll", titleKey: "create.enroll", aliasKey: "alias.enroll" },
  { moduleId: "students", kind: "create", key: "transfer", titleKey: "create.transfer", aliasKey: "alias.transfer" },
  // Ventanilla shortcut (Phase 7) — lands on the payment form, not just the surface.
  { moduleId: "finance", kind: "create", titleKey: "create.payment", aliasKey: "alias.payment", intent: "payment" },
  { moduleId: "grades", kind: "create", titleKey: "create.evaluation", aliasKey: "alias.evaluation" },
  // Grade-capture surface (Phase 5) — fewest-clicks shortcut into the roster grid.
  { moduleId: "grades", kind: "create", key: "capture", titleKey: "create.captureGrades", aliasKey: "alias.captureGrades" },
  { moduleId: "announcements", kind: "create", titleKey: "create.announcement", aliasKey: "alias.announcement" },
  { moduleId: "scholarships", kind: "create", titleKey: "create.scholarship", aliasKey: "alias.scholarship" },
  { moduleId: "documents", kind: "create", titleKey: "create.document", aliasKey: "alias.document" },
  { moduleId: "uniforms", kind: "create", titleKey: "create.uniform", aliasKey: "alias.uniform" },
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

/** Split a comma-joined localized alias string into trimmed terms. */
function parseAliases(value: string): string[] {
  return value
    .split(",")
    .map((term) => term.trim())
    .filter((term) => term.length > 0);
}

export function buildCommandRegistry(flags: FlagSet, t: Translate): Command[] {
  const commands: Command[] = [];

  for (const module of MODULES) {
    if (module.hidden) continue; // deactivated from the product → never surfaced.
    if (flags[module.flag] !== true) continue; // flag OFF → module contributes nothing.
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
    if (module === undefined || module.hidden || flags[module.flag] !== true) continue;
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

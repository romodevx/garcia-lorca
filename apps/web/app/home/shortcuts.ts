import type { FlagSet } from "@garcia-lorca/ui/flags";

import { MODULES } from "../_design/data";
import { ACTIONS, holdsAll } from "../command/registry";

/**
 * A card's shortcuts (PLAN-redesign-home §3.2): the registry's actions that
 * carry an intent, so ⌘K and the home offer the same list from ONE source. An
 * action without an intent only repeats the card (D10) and is left out.
 */
export interface HomeShortcut {
  readonly moduleId: string;
  readonly intent: string;
  /** Key in the `palette` namespace, the same title ⌘K shows. */
  readonly titleKey: string;
}

const flagOf = new Map(MODULES.map((m) => [m.id, m.flag]));

/**
 * The shortcuts `moduleId`'s card offers this caller: its flag on (an intent
 * into a disabled module lands on a placeholder) and every write permission the
 * action `requires` held (E4). UX only — the gateway stays the boundary.
 */
export function homeShortcuts(moduleId: string, permissions: readonly string[], flags: FlagSet): HomeShortcut[] {
  const flag = flagOf.get(moduleId);
  if (flag === undefined || flags[flag] !== true) return [];
  return ACTIONS.flatMap((action) =>
    action.moduleId === moduleId && action.intent !== undefined && holdsAll(action.requires, permissions)
      ? [{ moduleId, intent: action.intent, titleKey: action.titleKey }]
      : [],
  );
}

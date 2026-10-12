import type { FlagSet } from "@garcia-lorca/ui/flags";
import { describe, expect, it } from "vitest";

import { MODULES } from "../_design/data";
import { ACTIONS } from "../command/registry";
import { homeShortcuts } from "./shortcuts";

const ALL_ON: FlagSet = Object.fromEntries(MODULES.map((m) => [m.flag, true]));
const WRITER = ["students:create", "teachers:write", "expenses:write", "schedule:write", "activities:write", "scholarships:write"];
const intents = (moduleId: string, perms: readonly string[], flags: FlagSet = ALL_ON): string[] =>
  homeShortcuts(moduleId, perms, flags).map((s) => s.intent);

describe("home shortcuts (PLAN-redesign-home §3.2, §5)", () => {
  it("are the registry's actions that carry an intent — one list, not two", () => {
    const fromRegistry = ACTIONS.filter((a) => a.intent !== undefined).map((a) => `${a.moduleId}:${a.intent}`);
    const fromHome = MODULES.flatMap((m) => homeShortcuts(m.id, WRITER, ALL_ON).map((s) => `${s.moduleId}:${s.intent}`));
    expect(fromHome.sort()).toEqual(fromRegistry.sort());
  });

  it("name themselves with the palette's own title key", () => {
    expect(homeShortcuts("expenses", WRITER, ALL_ON)).toEqual([{ moduleId: "expenses", intent: "new", titleKey: "create.expense" }]);
  });

  // D10: a card whose shortcuts would only repeat the card gets none (no ⋯).
  it.each(["payments", "attendance", "grades", "announcements", "settings", "admin"])("%s has none", (id) => {
    expect(intents(id, WRITER)).toEqual([]);
  });

  // E4: a write shortcut is a promise the gateway breaks for a read-only role.
  it("drops a shortcut whose write permission the caller lacks", () => {
    expect(intents("expenses", ["expenses:read"])).toEqual([]);
    expect(intents("students", ["students:read"])).toEqual(["search"]);
    expect(intents("students", ["students:read", "students:create"])).toEqual(["new", "search"]);
  });

  it("drops every shortcut of a module whose flag is off", () => {
    expect(intents("expenses", WRITER, { ...ALL_ON, module_expenses: false })).toEqual([]);
  });
});

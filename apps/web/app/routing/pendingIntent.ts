import { useState } from "react";

/**
 * Shortcuts that land INSIDE a module ("Registrar egreso" opens Egresos on the
 * new-expense drawer), from ⌘K and from the home's quick actions.
 *
 * The shell's catch-all page remounts on every new path, so React state cannot
 * carry the intent across the navigation. It is handed over in module scope,
 * the way "Cubrir ahora" hands Horarios its dates (schedule/coverFocus.ts), and
 * the destination screen takes it once, on mount. It never goes in the URL:
 * back, F5 and a shared link open the plain screen.
 */

/** The intents each destination screen understands. */
export interface IntentOf {
  students: "new" | "search";
  teachers: "new";
  finance: "overdue";
  expenses: "new";
  schedule: "absence";
  activities: "new";
  scholarships: "new";
  uniforms: "inventory";
}

export type IntentModule = keyof IntentOf;

const KNOWN: { readonly [M in IntentModule]: readonly IntentOf[M][] } = {
  students: ["new", "search"],
  teachers: ["new"],
  finance: ["overdue"],
  expenses: ["new"],
  schedule: ["absence"],
  activities: ["new"],
  scholarships: ["new"],
  uniforms: ["inventory"],
};

type Pending = { [M in IntentModule]: { module: M; intent: IntentOf[M] } }[IntentModule];

let pending: Pending | undefined;

/** Ask the next `module` screen that mounts to open on `intent`. */
export function requestIntent<M extends IntentModule>(module: M, intent: IntentOf[M]): void {
  pending = { module, intent } as Pending;
}

/** Forget any pending intent: a plain open is a plain visit. */
export function clearIntent(): void {
  pending = undefined;
}

/**
 * The pending intent for `module`, once. Whatever screen takes first clears it,
 * so an intent whose screen never mounted (its module was off) cannot fire on
 * a later, unrelated visit.
 */
export function takeIntent<M extends IntentModule>(module: M): IntentOf[M] | undefined {
  const got = pending;
  pending = undefined;
  return got?.module === module ? (got.intent as IntentOf[M]) : undefined;
}

/** Validates a `module:<id>:<intent>` route's parts against what the screen understands. */
export function toIntent(module: string, intent: string | undefined): Pending | undefined {
  if (intent === undefined || !Object.hasOwn(KNOWN, module)) return undefined;
  const known: readonly string[] = KNOWN[module as IntentModule];
  return known.includes(intent) ? ({ module, intent } as Pending) : undefined;
}

/**
 * What the shell calls on every open: arms the intent when the route carries
 * one the screen understands, and otherwise disarms any leftover, so a plain
 * open is always a plain visit.
 */
export function handOffIntent(module: string, intent: string | undefined): void {
  pending = toIntent(module, intent);
}

/** The screen's intent, taken once on mount (survives React's double render in dev). */
export function useIntent<M extends IntentModule>(module: M): IntentOf[M] | undefined {
  const [intent] = useState(() => takeIntent(module));
  return intent;
}

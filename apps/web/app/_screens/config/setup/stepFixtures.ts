import { STEPS, type StepDef, type StepId, type StepStatus, type StepView } from "./setupSteps";

/** Test-only: the eight steps, all pending unless a step says otherwise. */
export function stepViews(overrides: Partial<Record<StepId, Partial<StepView>>> = {}, list: readonly StepDef[] = STEPS): StepView[] {
  return list.map((def): StepView => ({
    ...def,
    status: "pending" as StepStatus,
    line: `Falta ${def.title}`,
    next: def.number === list.length ? "none" : "text",
    ...overrides[def.id],
  }));
}

export function stepView(id: StepId, override: Partial<StepView> = {}): StepView {
  const found = stepViews({ [id]: override }).find((s) => s.id === id);
  if (found === undefined) throw new Error(`unknown step ${id}`);
  return found;
}

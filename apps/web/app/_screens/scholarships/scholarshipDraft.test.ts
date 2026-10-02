import { describe, expect, it } from "vitest";

import type { Scholarship } from "../../scholarships/api";
import { draftFromScholarship, emptyDraft, readDraft, valueError, type ScholarshipDraft } from "./scholarshipDraft";

const filled: ScholarshipDraft = { name: "BECA HERMANOS", kind: "percent", value: "50", targets: ["tuition"], active: true };

describe("scholarship draft (T18)", () => {
  it("starts as a percent, active, with nothing chosen", () => {
    expect(emptyDraft()).toEqual({ name: "", kind: "percent", value: "", targets: [], active: true });
  });

  it("reads a percent into basis points", () => {
    expect(readDraft(filled)).toEqual({ input: { name: "BECA HERMANOS", kind: "percent", value: 5000, targets: ["tuition"], active: true }, errors: {} });
  });

  it("reads pesos into cents for the fixed kinds", () => {
    expect(readDraft({ ...filled, kind: "fixed_off", value: "1,500.50" }).input?.value).toBe(150050);
    expect(readDraft({ ...filled, kind: "fixed_price", value: "0" }).input?.value).toBe(0);
  });

  it("says, field by field, what is missing", () => {
    const { input, errors } = readDraft(emptyDraft());
    expect(input).toBeUndefined();
    expect(errors).toEqual({ name: "Falta este dato", value: "Falta este dato", targets: "Elige al menos a qué se aplica." });
  });

  it.each([
    ["percent", "0", "El porcentaje va de 0.01 a 100."],
    ["percent", "100.5", "El porcentaje va de 0.01 a 100."],
    ["percent", "12,5", "Escribe el porcentaje con punto decimal y hasta dos decimales."],
    ["fixed_off", "0", "El descuento debe ser mayor a cero."],
    ["fixed_off", "abc", "Escribe un monto en pesos, por ejemplo 500.00."],
    ["fixed_price", "-1", "Escribe un monto en pesos, por ejemplo 500.00."],
  ] as const)("%s %s is refused", (kind, value, message) => {
    expect(valueError(kind, value)).toBe(message);
    expect(readDraft({ ...filled, kind, value }).errors.value).toBe(message);
  });

  it("an empty value is not 'badly written' — it is missing, said on save", () => {
    expect(valueError("percent", "")).toBeUndefined();
  });

  it("refuses more than 50 targets", () => {
    const targets = Array.from({ length: 51 }, (_, i) => `concept:00000000-0000-0000-0000-${String(i).padStart(12, "0")}`);
    expect(readDraft({ ...filled, targets }).errors.targets).toBe("Una beca se aplica a 50 cosas como máximo.");
  });

  it("edits a stored scholarship as typed text", () => {
    const stored: Scholarship = { id: "b1", name: "X", kind: "percent", value: 1250, targets: ["enrollment"], active: false, assignedCount: 3 };
    expect(draftFromScholarship(stored)).toEqual({ name: "X", kind: "percent", value: "12.5", targets: ["enrollment"], active: false });
    expect(draftFromScholarship({ ...stored, kind: "fixed_price", value: 100000 }).value).toBe("1000.00");
  });
});

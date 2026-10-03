import { describe, expect, it } from "vitest";

import {
  basisPointsToPercentInput,
  groupTargets,
  overlapWarning,
  percentToBasisPoints,
  scholarshipTitle,
  scholarshipValueLabel,
  targetsSummary,
} from "./format";
import type { ScholarshipTarget } from "./api";

const CONCEPT = "concept:7d1c2a4e-5b6f-4c3d-8e9f-0a1b2c3d4e5f";
const ACTIVITY = "activity:a1000000-0000-0000-0000-000000000001";
const catalog: ScholarshipTarget[] = [
  { key: "enrollment", label: "Inscripción", group: "enrollment" },
  { key: "tuition", label: "Colegiatura", group: "tuition" },
  { key: CONCEPT, label: "Cuota de materiales", group: "concept" },
  { key: ACTIVITY, label: "Natación", group: "activity" },
];

describe("scholarship format (T18)", () => {
  it.each([
    ["percent", 5000, "50 %"],
    ["percent", 1250, "12.5 %"],
    ["percent", 3333, "33.33 %"],
    ["percent", 10000, "100 %"],
    ["fixed_off", 50000, "$500.00 de descuento"],
    ["fixed_price", 100000, "Paga $1,000.00"],
    ["fixed_price", 0, "Paga $0.00"],
  ] as const)("%s %d reads %s", (kind, value, text) => {
    expect(scholarshipValueLabel(kind, value)).toBe(text);
  });

  it.each([
    ["50", 5000],
    ["12.5", 1250],
    ["33.33", 3333],
    [" 100 ", 10000],
    ["50 %", 5000],
    ["0", 0],
  ])("percent %s is %d basis points", (text, bps) => {
    expect(percentToBasisPoints(text)).toBe(bps);
  });

  it.each(["", "33.333", "12,5", "-5", "abc", "1e2"])("percent %s is not a percent", (text) => {
    expect(percentToBasisPoints(text)).toBeNaN();
  });

  it.each([
    [5000, "50"],
    [1250, "12.5"],
    [3333, "33.33"],
  ])("basis points %d edit as %s", (bps, text) => {
    expect(basisPointsToPercentInput(bps)).toBe(text);
  });

  it("names the targets in catalog order, and says so of one no longer offered", () => {
    expect(targetsSummary(["tuition", ACTIVITY, "enrollment"], catalog)).toBe("Inscripción, Colegiatura, Natación");
    expect(targetsSummary(["concept:00000000-0000-0000-0000-000000000009"], catalog)).toBe("Concepto retirado");
    expect(targetsSummary([], catalog)).toBe("—");
  });

  it("groups the targets Inscripción / Colegiatura / Conceptos / Actividades, empty groups left out", () => {
    const groups = groupTargets(catalog.filter((t) => t.group !== "concept"));
    expect(groups.map((g) => g.label)).toEqual(["Inscripción", "Colegiatura", "Actividades"]);
    expect(groups[2]?.targets.map((t) => t.label)).toEqual(["Natación"]);
  });

  it("calls a scholarship 'Beca …' once, never 'Beca BECA …'", () => {
    expect(scholarshipTitle("Excelencia")).toBe("Beca Excelencia");
    expect(scholarshipTitle("BECA HERMANOS")).toBe("BECA HERMANOS");
    expect(scholarshipTitle("")).toBe("Beca");
  });

  it("warns that overlapping scholarships do not stack", () => {
    expect(overlapWarning([{ scholarshipId: "x", name: "BECA EXCELENCIA" }])).toBe(
      "También tiene: BECA EXCELENCIA — no se acumulan; se aplica la más favorable",
    );
    expect(overlapWarning([{ scholarshipId: "x", name: "Excelencia" }, { scholarshipId: "y", name: "Deportiva" }])).toBe(
      "También tiene: Beca Excelencia, Beca Deportiva — no se acumulan; se aplica la más favorable",
    );
  });
});

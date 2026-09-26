// Regression: ISSUE-001 — a keyboard user tabbing onto a form field saw no focus ring
// Found by /qa on 2026-09-25
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-25-t1.md
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SelectField, TextField } from "./fields";
import { inputStyle } from "./Field";

describe("inputStyle keeps the app's focus ring", () => {
  // globals.css draws the ring with `:focus-visible { outline: … }`. An inline
  // `outline: none` beats any stylesheet rule, so the field went blank on Tab
  // (WCAG 2.4.7). The field must leave `outline` alone, valid or not.
  it.each([false, true])("does not set outline (invalid=%s)", (invalid) => {
    expect(inputStyle(invalid)).not.toHaveProperty("outline");
  });

  it("a SelectField carries no inline outline", () => {
    render(<SelectField label="Asignatura" value="" options={[{ value: "a", label: "A" }]} onChange={() => undefined} />);
    expect(screen.getByLabelText("Asignatura").style.outline).toBe("");
  });

  it("a TextField carries no inline outline", () => {
    render(<TextField label="Nombre" value="" onChange={() => undefined} />);
    expect(screen.getByLabelText("Nombre").style.outline).toBe("");
  });
});

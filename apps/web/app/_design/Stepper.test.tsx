import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Stepper } from "./Stepper";

const STEPS = [
  { id: "upload", label: "Subir" },
  { id: "preview", label: "Vista previa" },
  { id: "result", label: "Resultado" },
] as const;

describe("Stepper", () => {
  it("numbers every step in order and marks only the current one", () => {
    render(<Stepper steps={STEPS} current="preview" tone="forest" />);

    const items = within(screen.getByRole("list", { name: "Pasos" })).getAllByRole("listitem");
    expect(items.map((li) => li.textContent)).toEqual(["1Subir", "2Vista previa", "3Resultado"]);
    expect(items[1]).toHaveAttribute("aria-current", "step");
    expect(items[0]).not.toHaveAttribute("aria-current");
    expect(items[2]).not.toHaveAttribute("aria-current");
  });

  it("paints the current step in the tone it is given", () => {
    const { rerender } = render(<Stepper steps={STEPS} current="upload" tone="red" />);
    expect(screen.getAllByRole("listitem")[0].style.border).toContain("var(--red)");

    rerender(<Stepper steps={STEPS} current="upload" tone="forest" />);
    expect(screen.getAllByRole("listitem")[0].style.border).toContain("var(--forest)");
  });
});

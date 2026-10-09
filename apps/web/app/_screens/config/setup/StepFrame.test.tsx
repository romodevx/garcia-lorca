import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { StepFrame } from "./StepFrame";
import { stepView } from "./stepFixtures";

/** A Btn's look is its inline background: lime is the only `var(--primary)` one. */
const isLime = (el: HTMLElement): boolean => el.style.background === "var(--primary)";

describe("StepFrame (§1.2, §1.3)", () => {
  it("says where the step stands and leaves title and purpose to the panel's own heading", () => {
    render(<StepFrame step={stepView("levels")} onGo={() => {}}><p>cuerpo</p></StepFrame>);
    expect(screen.getByText("Paso 3 de 8")).toBeInTheDocument();
    expect(screen.getByText("cuerpo")).toBeInTheDocument();
    expect(screen.queryByRole("heading")).toBeNull();
    // The panel's subtitle already says what the step is for; saying it twice was QA's first finding.
    expect(screen.queryByText("Los niveles que ofrece el colegio y los grados de cada uno.")).toBeNull();
  });

  it("titles the step and says what it is for only when its body has no panel to do it", () => {
    render(<StepFrame step={stepView("terms")} ownHeading onGo={() => {}}>x</StepFrame>);
    const heading = screen.getByRole("heading", { level: 2, name: "Periodos" });
    expect(heading).toHaveAttribute("tabindex", "-1");
    expect(screen.getByText(stepView("terms").purpose)).toBeInTheDocument();
  });

  it("goes back and forward by the neighbours' names", async () => {
    const onGo = vi.fn();
    const user = userEvent.setup();
    render(<StepFrame step={stepView("groups", { next: "primary" })} onGo={onGo}>x</StepFrame>);

    await user.click(screen.getByRole("button", { name: "Anterior: Materias" }));
    await user.click(screen.getByRole("button", { name: "Siguiente: Colegiaturas" }));
    expect(onGo.mock.calls).toEqual([["subjects"], ["tuition"]]);
  });

  it("has no way back on step 1", () => {
    render(<StepFrame step={stepView("profile")} onGo={() => {}}>x</StepFrame>);
    expect(screen.queryByRole("button", { name: /^Anterior/ })).toBeNull();
    expect(screen.getByRole("button", { name: "Siguiente: Ciclo escolar" })).toBeInTheDocument();
  });

  it("has no Siguiente on step 8", () => {
    render(<StepFrame step={stepView("calendar", { next: "none" })} onGo={() => {}}>x</StepFrame>);
    expect(screen.queryByRole("button", { name: /^Siguiente/ })).toBeNull();
    expect(screen.getByRole("button", { name: "Anterior: Colegiaturas" })).toBeInTheDocument();
  });

  it("makes Siguiente lime only when the step has no lime of its own", () => {
    const { unmount } = render(<StepFrame step={stepView("groups", { next: "primary" })} onGo={() => {}}>x</StepFrame>);
    expect(isLime(screen.getByRole("button", { name: "Siguiente: Colegiaturas" }))).toBe(true);
    unmount();

    render(<StepFrame step={stepView("levels", { next: "text" })} onGo={() => {}}>x</StepFrame>);
    expect(isLime(screen.getByRole("button", { name: "Siguiente: Periodos" }))).toBe(false);
  });

  it("is hidden, not unmounted, when it is not the open step (§3.1)", () => {
    render(<StepFrame step={stepView("levels")} hidden onGo={() => {}}><button type="button">dentro</button></StepFrame>);
    expect(screen.queryByRole("button", { name: "dentro" })).toBeNull();
    expect(screen.getByRole("button", { name: "dentro", hidden: true })).not.toBeVisible();
  });

  it("draws only its body while prices take the screen", () => {
    render(<StepFrame step={stepView("tuition")} chrome={false} onGo={() => {}}><p>tabulador</p></StepFrame>);
    expect(screen.getByText("tabulador")).toBeInTheDocument();
    expect(screen.queryByText("Paso 7 de 8")).toBeNull();
    expect(screen.queryByRole("button", { name: /Siguiente|Anterior/ })).toBeNull();
  });

  it("frames the Papelera with no step label and no footer (§1.5)", () => {
    render(<StepFrame tool onGo={() => {}}><p>papelera</p></StepFrame>);
    expect(screen.getByText("papelera")).toBeInTheDocument();
    expect(screen.queryByText(/^Paso/)).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

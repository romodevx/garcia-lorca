import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SetupStepList, type SetupStepListProps } from "./SetupStepList";
import { stepViews } from "./stepFixtures";

function renderSteps(props: Partial<SetupStepListProps> = {}): SetupStepListProps {
  const all: SetupStepListProps = {
    steps: stepViews(),
    cardTitle: "Puesta en marcha",
    openId: "groups",
    onOpen: vi.fn(),
    showTrash: false,
    narrow: false,
    ...props,
  };
  render(<SetupStepList {...all} />);
  return all;
}

const nav = (): HTMLElement => screen.getByRole("navigation", { name: "Puesta en marcha" });
const stepButton = (name: RegExp): HTMLElement => within(nav()).getByRole("button", { name });

describe("SetupStepList — the list (§1.1, §1.7)", () => {
  it("lists the eight steps as buttons and marks the open one with aria-current=step", () => {
    renderSteps();
    expect(within(nav()).getAllByRole("button")).toHaveLength(8);
    expect(stepButton(/^Grupos/)).toHaveAttribute("aria-current", "step");
    expect(stepButton(/^Periodos/)).not.toHaveAttribute("aria-current");
  });

  it("opens the step that is clicked", async () => {
    const user = userEvent.setup();
    const props = renderSteps();
    await user.click(stepButton(/^Colegiaturas/));
    expect(props.onOpen).toHaveBeenCalledWith("tuition");
  });

  it("titles the card with what it is given", () => {
    renderSteps({ cardTitle: "Configuración" });
    expect(screen.getByRole("navigation", { name: "Configuración" })).toBeInTheDocument();
  });

  it("shows each step's line, and keeps the full text in title for when it is cut (§1.6)", () => {
    renderSteps({ steps: stepViews({ groups: { line: "Sin grupos en 2° de SECUNDARIA" } }) });
    expect(stepButton(/^Grupos/)).toHaveTextContent("Sin grupos en 2° de SECUNDARIA");
    expect(stepButton(/^Grupos/)).toHaveAttribute("title", "Grupos · Sin grupos en 2° de SECUNDARIA");
  });
});

describe("SetupStepList — each status (§4.3)", () => {
  it("a done step shows a check and its line", () => {
    renderSteps({ steps: stepViews({ profile: { status: "done", line: "GARCÍA LORCA" } }) });
    const step = stepButton(/^Datos del colegio/);
    expect(step).toHaveTextContent("GARCÍA LORCA");
    expect(step.querySelector("[data-mark='done']")).not.toBeNull();
  });

  it("a step still loading shows its number in grey and 'Revisando…', no spinner", () => {
    renderSteps({ steps: stepViews({ terms: { status: "loading", line: "Revisando…" } }) });
    const step = stepButton(/^Periodos/);
    expect(step).toHaveTextContent("4");
    expect(step).toHaveTextContent("Revisando…");
    expect(step.querySelector("[data-mark='loading']")).not.toBeNull();
  });

  it("a failed read leaves the step with no mark and no line", () => {
    renderSteps({ steps: stepViews({ groups: { status: "unknown", line: undefined } }) });
    const step = stepButton(/^Grupos/);
    expect(step.querySelector("[data-line]")).toBeNull();
    expect(step.querySelector("[data-mark='done']")).toBeNull();
  });

  it("a pending step says what is missing in the warning colour", () => {
    renderSteps({ steps: stepViews({ tuition: { line: "Ningún nivel tiene precio" } }) });
    const line = stepButton(/^Colegiaturas/).querySelector<HTMLElement>("[data-line]");
    expect(line).toHaveTextContent("Ningún nivel tiene precio");
    expect(line?.style.color).toBe("var(--warn-fg)");
  });

  it("the open step gets the lime circle", () => {
    renderSteps();
    const circle = stepButton(/^Grupos/).querySelector<HTMLElement>("[data-mark]");
    expect(circle?.style.background).toBe("var(--lime)");
  });
});

describe("SetupStepList — Herramientas (§1.5)", () => {
  it("offers Migrar alumnos once, with what it needs, only when the caller may", async () => {
    const onOpenStudentImport = vi.fn();
    const user = userEvent.setup();
    renderSteps({ onOpenStudentImport });

    const migrate = screen.getByRole("button", { name: /Migrar alumnos del sistema anterior/ });
    expect(migrate).toHaveTextContent("Necesita niveles, grados y grupos.");
    await user.click(migrate);
    expect(onOpenStudentImport).toHaveBeenCalledTimes(1);
    expect(screen.getAllByRole("button", { name: /Migrar alumnos/ })).toHaveLength(1);
  });

  it("shows neither tool without its permission", () => {
    renderSteps();
    expect(screen.queryByText("Herramientas")).toBeNull();
    expect(screen.queryByRole("button", { name: /Migrar alumnos/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Papelera/ })).toBeNull();
  });

  it("opens the Papelera in the right column, with no count, and then no step is open", async () => {
    const user = userEvent.setup();
    const props = renderSteps({ showTrash: true });
    const trash = screen.getByRole("button", { name: "Papelera" });
    await user.click(trash);
    expect(props.onOpen).toHaveBeenCalledWith("trash");
  });

  it("marks the Papelera as the open row while it is open", () => {
    renderSteps({ showTrash: true, openId: "trash" });
    expect(screen.getByRole("button", { name: "Papelera" })).toHaveAttribute("aria-current", "true");
    for (const step of within(nav()).getAllByRole("button")) expect(step).not.toHaveAttribute("aria-current");
  });
});

describe("SetupStepList — narrow screens (§1.6)", () => {
  it("collapses the list into a bar that names the open step", () => {
    renderSteps({ narrow: true });
    const bar = screen.getByRole("button", { name: "Paso 6 de 8 · Grupos" });
    expect(bar).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("navigation")).toBeNull();
  });

  it("opens the list from the bar, and choosing a step closes it", async () => {
    const user = userEvent.setup();
    const props = renderSteps({ narrow: true });
    const bar = screen.getByRole("button", { name: "Paso 6 de 8 · Grupos" });

    await user.click(bar);
    expect(bar).toHaveAttribute("aria-expanded", "true");
    await user.click(stepButton(/^Calendario/));

    expect(props.onOpen).toHaveBeenCalledWith("calendar");
    expect(bar).toHaveAttribute("aria-expanded", "false");
  });

  it("closes on Escape and gives focus back to the bar", async () => {
    const user = userEvent.setup();
    renderSteps({ narrow: true });
    const bar = screen.getByRole("button", { name: "Paso 6 de 8 · Grupos" });

    await user.click(bar);
    stepButton(/^Periodos/).focus();
    await user.keyboard("{Escape}");

    expect(bar).toHaveAttribute("aria-expanded", "false");
    expect(bar).toHaveFocus();
  });

  it("names the Papelera in the bar while it is open", () => {
    renderSteps({ narrow: true, showTrash: true, openId: "trash" });
    expect(screen.getByRole("button", { name: "Papelera" })).toHaveAttribute("aria-expanded", "false");
  });
});

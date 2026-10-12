import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { TargetSection } from "./audience";
import { GroupedTargets } from "./GroupedTargets";

function section(title: string | undefined, ...ids: string[]): TargetSection {
  return { key: title ?? "flat", title, targets: ids.map((id) => ({ id, label: [id, title, "· full"].filter(Boolean).join(" "), text: id })) };
}

const TWO_LEVELS = [section("Primaria", "1° A", "1° B"), section("Secundaria", "1° A s")];

function renderTargets(sections: TargetSection[], picked: string[] = [], loaded = true) {
  const onToggle = vi.fn();
  render(<GroupedTargets sections={sections} picked={picked} onToggle={onToggle} loaded={loaded} emptyText="Este ciclo no tiene grupos." />);
  return onToggle;
}

describe("GroupedTargets", () => {
  // D11
  it("says it is loading until the list arrives", () => {
    renderTargets([], [], false);
    expect(screen.getByText("Cargando…")).toBeInTheDocument();
  });

  it("says so when the list arrived empty", () => {
    renderTargets([]);
    expect(screen.getByText("Este ciclo no tiene grupos.")).toBeInTheDocument();
  });

  it("files each level's rows under a disclosure titled by the level, with today's names", () => {
    renderTargets(TWO_LEVELS);
    const primaria = screen.getByText("Primaria").closest("details");
    expect(primaria).not.toBeNull();
    expect(primaria).toContainElement(screen.getByRole("button", { name: "1° A Primaria · full" }));
    expect(screen.getByRole("button", { name: "1° A Primaria · full" })).toHaveTextContent("1° A");
  });

  it("counts a level's groups, and how many of them are picked", () => {
    renderTargets(TWO_LEVELS, ["1° B"]);
    expect(screen.getByText("Primaria").closest("summary")).toHaveTextContent("1 de 2");
    expect(screen.getByText("Secundaria").closest("summary")).toHaveTextContent("1");
  });

  it("opens every level when the school has three or fewer", () => {
    const { container } = render(<GroupedTargets sections={TWO_LEVELS} picked={[]} onToggle={() => {}} loaded emptyText="" />);
    expect([...container.querySelectorAll("details")].map((d) => d.open)).toEqual([true, true]);
  });

  it("keeps the levels closed when the school has more than three, except where something is picked", () => {
    const four = [section("Preescolar", "K"), section("Primaria", "P"), section("Secundaria", "S"), section("Bachillerato", "B")];
    const { container } = render(<GroupedTargets sections={four} picked={["S"]} onToggle={() => {}} loaded emptyText="" />);
    expect([...container.querySelectorAll("details")].map((d) => d.open)).toEqual([false, false, true, false]);
  });

  it("is a plain list when its only section has no title", () => {
    const { container } = render(<GroupedTargets sections={[section(undefined, "Primaria", "Secundaria")]} picked={["Primaria"]} onToggle={() => {}} loaded emptyText="" />);
    expect(container.querySelector("details")).toBeNull();
    expect(screen.getByRole("button", { name: "Primaria · full" })).toHaveAttribute("aria-pressed", "true");
  });

  it("toggles the row's id", async () => {
    const onToggle = renderTargets(TWO_LEVELS);
    await userEvent.setup().click(screen.getByRole("button", { name: "1° B Primaria · full" }));
    expect(onToggle).toHaveBeenCalledWith("1° B");
  });
});

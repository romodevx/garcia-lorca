import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { TargetSection } from "./audience";
import { GroupedTargets } from "./GroupedTargets";

function section(title: string | undefined, ...ids: string[]): TargetSection {
  return { key: title ?? "flat", title, targets: ids.map((id) => ({ id, label: [id, title, "· full"].filter(Boolean).join(" "), text: id })) };
}

/** A level's header: a real button (ISSUE-T14-021), named by the level and its count. */
const level = (name: string): HTMLElement => screen.getByRole("button", { name: new RegExp(`^${name} · `) });
const expanded = (): (string | null)[] =>
  screen.queryAllByRole("button").filter((b) => b.hasAttribute("aria-expanded")).map((b) => b.getAttribute("aria-expanded"));

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
    const header = level("Primaria");
    const body = document.getElementById(header.getAttribute("aria-controls") ?? "");
    expect(body).not.toBeNull();
    expect(body).toContainElement(screen.getByRole("button", { name: "1° A Primaria · full" }));
    expect(screen.getByRole("button", { name: "1° A Primaria · full" })).toHaveTextContent("1° A");
  });

  it("counts a level's groups, and how many of them are picked", () => {
    renderTargets(TWO_LEVELS, ["1° B"]);
    expect(level("Primaria")).toHaveTextContent("1 de 2");
    expect(level("Secundaria")).toHaveTextContent("1");
  });

  it("opens every level when the school has three or fewer", () => {
    render(<GroupedTargets sections={TWO_LEVELS} picked={[]} onToggle={() => {}} loaded emptyText="" />);
    expect(expanded()).toEqual(["true", "true"]);
  });

  it("keeps the levels closed when the school has more than three, except where something is picked", () => {
    const four = [section("Preescolar", "K"), section("Primaria", "P"), section("Secundaria", "S"), section("Bachillerato", "B")];
    render(<GroupedTargets sections={four} picked={["S"]} onToggle={() => {}} loaded emptyText="" />);
    expect(expanded()).toEqual(["false", "false", "true", "false"]);
  });

  it("is a plain list when its only section has no title", () => {
    render(<GroupedTargets sections={[section(undefined, "Primaria", "Secundaria")]} picked={["Primaria"]} onToggle={() => {}} loaded emptyText="" />);
    expect(expanded()).toEqual([]);
    expect(screen.getByRole("button", { name: "Primaria · full" })).toHaveAttribute("aria-pressed", "true");
  });

  it("toggles the row's id", async () => {
    const onToggle = renderTargets(TWO_LEVELS);
    await userEvent.setup().click(screen.getByRole("button", { name: "1° B Primaria · full" }));
    expect(onToggle).toHaveBeenCalledWith("1° B");
  });

  // ISSUE-T14-021: the level header was a styled summary the QA could not reach as a button.
  it("opens and closes a level from the keyboard, saying whether it is open", async () => {
    const user = userEvent.setup();
    renderTargets(TWO_LEVELS);
    const header = level("Primaria");
    expect(header).toHaveAttribute("aria-expanded", "true");

    header.focus();
    await user.keyboard("{Enter}");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("button", { name: "1° A Primaria · full" })).toBeNull();

    await user.keyboard(" ");
    expect(header).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("button", { name: "1° A Primaria · full" })).toBeInTheDocument();
  });
});

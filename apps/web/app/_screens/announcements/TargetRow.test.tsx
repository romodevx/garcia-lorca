import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TargetRow } from "./TargetRow";

describe("TargetRow", () => {
  it("is a toggle button named by its label, whatever it shows", () => {
    render(<TargetRow label="1° PRIMARIA · A" text="1° A" on={false} onToggle={() => {}} mark="check" />);
    const row = screen.getByRole("button", { name: "1° PRIMARIA · A" });
    expect(row).toHaveAttribute("type", "button");
    expect(row).toHaveAttribute("aria-pressed", "false");
    expect(row).toHaveTextContent("1° A");
  });

  // D2: the ✓ is an icon, never text, so "Grupos" stays "Grupos".
  it("draws the check without changing the accessible name", () => {
    const { container } = render(<TargetRow label="Grupos" text="Grupos" on onToggle={() => {}} mark="check" />);
    const row = screen.getByRole("button", { name: "Grupos" });
    expect(row).toHaveAccessibleName("Grupos");
    expect(row).toHaveAttribute("aria-pressed", "true");
    expect(container.querySelector("[aria-hidden='true'] svg, svg[aria-hidden='true']")).not.toBeNull();
  });

  it("draws no check while off", () => {
    const { container } = render(<TargetRow label="1° A" text="1° A" on={false} onToggle={() => {}} mark="check" />);
    expect(container.querySelector("svg")).toBeNull();
  });

  it("shows a second line that stays out of the name", () => {
    render(<TargetRow label="Cruz Valentina" text="Cruz Valentina" detail="Primaria · 1° A" on={false} onToggle={() => {}} mark="radio" />);
    expect(screen.getByRole("button", { name: "Cruz Valentina" })).toHaveTextContent("Primaria · 1° A");
  });

  it("toggles on click", async () => {
    const onToggle = vi.fn();
    render(<TargetRow label="1° A" text="1° A" on={false} onToggle={onToggle} mark="check" />);
    await userEvent.setup().click(screen.getByRole("button", { name: "1° A" }));
    expect(onToggle).toHaveBeenCalledTimes(1);
  });
});

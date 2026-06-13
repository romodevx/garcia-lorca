import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MODULES, USERS } from "../_design/data";
import { Dashboard } from "./Dashboard";

describe("Dashboard", () => {
  it("renders a card for all 11 modules", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} now={new Date("2026-06-12T09:00:00")} />);

    expect(MODULES).toHaveLength(11);
    // Each module card surfaces its unique description text — assert one per module.
    for (const m of MODULES) {
      expect(screen.getByText(m.desc)).toBeInTheDocument();
    }
    // And exactly 11 module-launcher cards exist alongside the 3 KPI cards.
    const launcherCards = MODULES.map((m) => screen.getByText(m.label));
    expect(launcherCards).toHaveLength(11);
  });

  it("greets by time of day", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} now={new Date("2026-06-12T09:00:00")} />);
    expect(screen.getByRole("heading", { name: /Buenos días, Laura/ })).toBeInTheDocument();
  });

  it("navigates from a KPI card", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    render(<Dashboard user={USERS.admin} onOpen={onOpen} now={new Date("2026-06-12T09:00:00")} />);

    await user.click(screen.getByRole("button", { name: /Adeudos del mes/ }));
    expect(onOpen).toHaveBeenCalledWith("finance");
  });
});

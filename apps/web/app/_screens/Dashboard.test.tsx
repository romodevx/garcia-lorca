import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MODULES, USERS } from "../_design/data";
import { Dashboard } from "./Dashboard";

// The dashboard now reads real KPIs from the gateway — mock the clients so the
// component renders without hitting the network.
vi.mock("../students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));
vi.mock("../config/api", () => ({ listCycles: vi.fn().mockResolvedValue([]) }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn().mockResolvedValue([]) }));

const VISIBLE_MODULES = MODULES.filter((m) => !m.hidden);
const HIDDEN_MODULES = MODULES.filter((m) => m.hidden);

describe("Dashboard", () => {
  it("renders a launcher card for every visible module", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} now={new Date("2026-06-12T09:00:00")} />);

    for (const m of VISIBLE_MODULES) {
      expect(screen.getByText(m.desc)).toBeInTheDocument();
    }
    const launcherCards = VISIBLE_MODULES.map((m) => screen.getByText(m.label));
    expect(launcherCards).toHaveLength(VISIBLE_MODULES.length);
  });

  it("does not render modules deactivated from the panel (e.g. Documentos)", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} now={new Date("2026-06-12T09:00:00")} />);

    expect(HIDDEN_MODULES.some((m) => m.id === "documents")).toBe(true);
    for (const m of HIDDEN_MODULES) {
      expect(screen.queryByText(m.label)).not.toBeInTheDocument();
      expect(screen.queryByText(m.desc)).not.toBeInTheDocument();
    }
  });

  it("greets by time of day with the real user", () => {
    render(<Dashboard user={USERS.admin} onOpen={() => {}} now={new Date("2026-06-12T09:00:00")} />);
    expect(screen.getByRole("heading", { name: /Buenos días, Director/ })).toBeInTheDocument();
  });

  it("navigates from a KPI card", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    render(<Dashboard user={USERS.admin} onOpen={onOpen} now={new Date("2026-06-12T09:00:00")} />);

    await user.click(screen.getByRole("button", { name: /Por cobrar/ }));
    expect(onOpen).toHaveBeenCalledWith("finance");
  });
});

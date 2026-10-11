import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { clearIntent, requestIntent } from "../../routing/pendingIntent";
import * as api from "../../schedule/api";
import { sampleBoard } from "../../schedule/__fixtures__/board";
import { ScheduleEntry } from "./ScheduleEntry";

vi.mock("../../schedule/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../schedule/api")>()),
  getBoard: vi.fn(),
  getOwnBoard: vi.fn(),
  listUncovered: vi.fn(),
  listAbsences: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(api.getBoard).mockResolvedValue(sampleBoard());
  vi.mocked(api.getOwnBoard).mockResolvedValue(sampleBoard());
  vi.mocked(api.listUncovered).mockResolvedValue([]);
  vi.mocked(api.listAbsences).mockResolvedValue([]);
});
afterEach(() => {
  clearIntent();
  vi.clearAllMocks();
});

// Redesign F1: "Registrar ausencia" from ⌘K or the home lands ON the drawer.
describe("ScheduleEntry — opened from a shortcut", () => {
  it("opens the absence drawer for whoever manages the timetable", async () => {
    requestIntent("schedule", "absence");
    render(<ScheduleEntry enabled permissions={["schedule:read", "schedule:write"]} onBack={() => undefined} />);
    expect(await screen.findByRole("dialog", { name: /Registrar ausencia/ })).toBeInTheDocument();
  });

  it("never opens it for a reader of the timetable", async () => {
    requestIntent("schedule", "absence");
    render(<ScheduleEntry enabled permissions={["schedule:read"]} onBack={() => undefined} />);
    await waitFor(() => expect(api.getBoard).toHaveBeenCalled());
    await screen.findByRole("tab", { name: "Semana" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("never opens it on a teacher's own week", async () => {
    requestIntent("schedule", "absence");
    render(<ScheduleEntry enabled permissions={["teaching:read"]} onBack={() => undefined} />);
    await screen.findByRole("tab", { name: "Mi semana" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});

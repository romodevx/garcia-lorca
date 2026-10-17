import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import { CalendarPanel } from "./CalendarPanel";

vi.mock("../../config/api", async (importOriginal) => ({
  ConfigError: (await importOriginal<typeof import("../../config/api")>()).ConfigError,
  listNonWorkingPeriods: vi.fn(),
  createNonWorkingPeriod: vi.fn(),
  deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(),
  createSchoolEvent: vi.fn(),
  deleteSchoolEvent: vi.fn(),
}));
const config = vi.mocked(configApi);

const cycle = { id: "c1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-24", endsOn: "2027-07-09" };

beforeEach(() => {
  config.listSchoolEvents.mockResolvedValue([]);
  config.listNonWorkingPeriods.mockResolvedValue([
    { id: "per-1", cycleId: "c1", label: "REVOLUCIÓN", kind: "holiday", startsOn: "2026-11-16", endsOn: "2026-11-16" },
  ]);
});
afterEach(() => vi.clearAllMocks());

// QA CC-106: the 16th of November could be closed twice. The server now refuses
// it, and the screen says which entry already closes that day.
describe("CalendarPanel · a day already closed", () => {
  it("shows the server's sentence naming the period that already closes it", async () => {
    const sentence = 'Ese día ya está marcado sin clases: "REVOLUCIÓN" (16 de noviembre de 2026)';
    config.createNonWorkingPeriod.mockRejectedValue(new configApi.ConfigError(409, sentence, "NON_WORKING_OVERLAP"));
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} />);
    await screen.findByText("REVOLUCIÓN");

    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "Día de la Revolución");
    await user.type(screen.getByLabelText("Del día"), "2026-11-16");
    await user.keyboard("{Enter}");

    expect(await screen.findByText(sentence)).toBeInTheDocument();
  });
});

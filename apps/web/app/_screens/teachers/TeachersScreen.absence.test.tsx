import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as scheduleApi from "../../schedule/api";
import * as teachersApi from "../../teachers/api";
import { TeachersScreen } from "./TeachersScreen";

vi.mock("../../teachers/api", async (importOriginal) => ({
  ...(await importOriginal<typeof teachersApi>()),
  listTeachers: vi.fn(),
  listTeachingAssignments: vi.fn(),
}));
vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof configApi>()),
  listCycles: vi.fn(),
}));
vi.mock("../../schedule/api", async (importOriginal) => ({
  ...(await importOriginal<typeof scheduleApi>()),
  createAbsence: vi.fn(),
}));

const ADA = "a0000000-0000-4000-8000-000000000001";
const ada = { id: ADA, firstName: "Ada", lastName: "Lovelace", email: "ada@escuela.mx", userId: "" };

beforeEach(() => {
  vi.mocked(teachersApi.listTeachers).mockResolvedValue([ada]);
  vi.mocked(teachersApi.listTeachingAssignments).mockResolvedValue([]);
  vi.mocked(configApi.listCycles).mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("Docentes — registrar una ausencia desde la plantilla (T19)", () => {
  it("opens the drawer with the teacher chosen and offers to cover what is left", async () => {
    vi.mocked(scheduleApi.createAbsence).mockResolvedValue({
      absence: { id: "x", cycleId: "c", teacherId: ADA, startsOn: "2026-10-05", endsOn: "2026-10-05", allDay: true, startsAt: null, endsAt: null, cancelled: false },
      uncoveredCount: 4,
    });
    const onCover = vi.fn();
    const user = userEvent.setup();
    render(<TeachersScreen onBack={() => undefined} onOpenLoad={() => undefined} canRegisterAbsence onCoverClasses={onCover} />);
    await user.click(await screen.findByRole("button", { name: "Registrar ausencia de Ada Lovelace" }));
    const dialog = screen.getByRole("dialog", { name: /Registrar ausencia/ });
    expect(within(dialog).getByRole("combobox", { name: /^Docente/ })).toHaveValue(ADA);
    await user.click(within(dialog).getByRole("button", { name: "Vacaciones" }));
    await user.click(within(dialog).getByRole("button", { name: "Registrar ausencia" }));
    await waitFor(() => expect(scheduleApi.createAbsence).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ teacherId: ADA, kind: "vacation", allDay: true })));
    expect(vi.mocked(scheduleApi.createAbsence).mock.calls[0]?.[1]).not.toHaveProperty("cycleId");
    expect(await screen.findByText("Quedan 4 clases sin cubrir en esas fechas")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cubrir ahora" }));
    expect(onCover).toHaveBeenCalled();
  });

  it("is not offered without schedule:write (or with Horarios off)", async () => {
    render(<TeachersScreen onBack={() => undefined} onOpenLoad={() => undefined} />);
    await screen.findByText("Ada Lovelace");
    expect(screen.queryByRole("button", { name: /Registrar ausencia/ })).not.toBeInTheDocument();
  });
});

import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../activities/api";
import * as roster from "../../_design/students/useStudentRoster";
import { ActivityEnrollPage } from "./ActivityEnrollPage";

vi.mock("../../activities/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../activities/api")>();
  return { ...real, listEnrollments: vi.fn(), enrollStudents: vi.fn(), endEnrollments: vi.fn(), changeActivityOption: vi.fn() };
});
vi.mock("../../_design/students/useStudentRoster", () => ({ useStudentRoster: vi.fn() }));
const m = vi.mocked(api);

const ACT = "a1000000-0000-0000-0000-000000000001";
const ONE = "0f000000-0000-0000-0000-000000000001";
const FLAT = "0f000000-0000-0000-0000-000000000002";
const swim: api.Activity = {
  id: ACT, name: "Natación", enrolledCount: 1, periodicity: "monthly", active: true, lateFeeCents: 0,
  schedule: [{ weekday: 2, start: "15:00", end: "16:30" }], billingStartsOn: "2026-10-01", firstDueOn: "2026-10-10",
  options: [{ id: FLAT, daysPerWeek: null, amountCents: 90000 }, { id: ONE, daysPerWeek: 1, amountCents: 60000 }],
};

const pupils: roster.RosterStudent[] = [
  { id: "s1", lastName: "Arce", firstName: "Ana", levelId: "pri", gradeLevelId: "", groupId: "", room: "1° PRIMARIA · A" },
  { id: "s2", lastName: "Bello", firstName: "Beto", levelId: "pri", gradeLevelId: "", groupId: "", room: "" },
  { id: "s3", lastName: "Cano", firstName: "Ceci", levelId: "pri", gradeLevelId: "", groupId: "", room: "" },
];

beforeEach(() => {
  vi.mocked(roster.useStudentRoster).mockReturnValue({ status: "ok", students: pupils, catalog: { levels: [{ id: "pri", name: "Primaria" }], grades: [], groups: [] } });
  m.listEnrollments.mockResolvedValue([{ id: "e3", activityId: ACT, studentId: "s3", optionId: FLAT, startsOn: "2026-10-01" }]);
});
afterEach(() => vi.clearAllMocks());

function open(canWrite = true): void {
  render(<ActivityEnrollPage activity={swim} canWrite={canWrite} onBack={() => undefined} />);
}

const enrolled = (): HTMLElement => screen.getByRole("region", { name: "Inscritos" });

describe("ActivityEnrollPage (T17)", () => {
  it("shows who is enrolled with their option, and the rest to pick from", async () => {
    open();
    expect(await within(enrolled()).findByText("Cano Ceci")).toBeInTheDocument();
    expect(within(enrolled()).getByLabelText("Esquema de Cano Ceci")).toHaveValue(FLAT);
    expect(screen.getByText("1° PRIMARIA · A")).toBeInTheDocument();
  });

  it("enrolls the marked pupils with the batch's option and says how many charges were issued", async () => {
    const user = userEvent.setup();
    m.enrollStudents.mockResolvedValue({ enrollments: [
      { id: "e1", activityId: ACT, studentId: "s1", optionId: ONE, startsOn: "2026-10-02" },
      { id: "e2", activityId: ACT, studentId: "s2", optionId: ONE, startsOn: "2026-10-02" },
    ], chargesCreated: 2 });
    open();
    await within(enrolled()).findByText("Cano Ceci");
    // Pre-filled with the cheapest scheme by days; the office picks another.
    const batch = screen.getByLabelText("Esquema de cobro");
    expect(batch).toHaveValue(ONE);
    await user.selectOptions(batch, "1 día/semana — $600.00");
    await user.click(screen.getByRole("checkbox", { name: "Arce Ana" }));
    await user.click(screen.getByRole("checkbox", { name: "Bello Beto" }));
    await user.click(screen.getByRole("button", { name: /Inscribir/ }));

    // Charges are issued at once and survive an unenrolment: it asks first.
    const dialog = await screen.findByRole("dialog", { name: "Inscribir alumnos" });
    expect(dialog).toHaveTextContent("¿Inscribir 2 alumnos a Natación con 1 día/semana — $600.00?");
    expect(dialog).toHaveTextContent(/no se borra al darlos de baja/);
    expect(m.enrollStudents).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole("button", { name: "Inscribir" }));

    expect(m.enrollStudents).toHaveBeenCalledWith(expect.any(String), ACT, [{ studentId: "s1", optionId: ONE }, { studentId: "s2", optionId: ONE }]);
    expect(await screen.findByText("2 alumnos inscritos · 2 cargos generados")).toBeInTheDocument();
    expect(within(enrolled()).getByLabelText("Esquema de Arce Ana")).toHaveValue(ONE);
  });

  it("cancelling the confirmation enrolls nobody and keeps the pupil marked", async () => {
    const user = userEvent.setup();
    open();
    await within(enrolled()).findByText("Cano Ceci");
    await user.click(screen.getByRole("checkbox", { name: "Arce Ana" }));
    await user.click(screen.getByRole("button", { name: /Inscribir/ }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("¿Inscribir 1 alumno a Natación");
    await user.click(within(dialog).getByRole("button", { name: "Cancelar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(m.enrollStudents).not.toHaveBeenCalled();
    expect(screen.getByRole("checkbox", { name: "Arce Ana" })).toBeChecked();
  });

  it("changes one pupil's option on the spot", async () => {
    const user = userEvent.setup();
    m.changeActivityOption.mockResolvedValue({ id: "e3", activityId: ACT, studentId: "s3", optionId: ONE, startsOn: "2026-10-01" });
    open();
    const select = await within(enrolled()).findByLabelText("Esquema de Cano Ceci");
    await user.selectOptions(select, "1 día/semana — $600.00");
    expect(m.changeActivityOption).toHaveBeenCalledWith(expect.any(String), ACT, "s3", ONE);
    expect(await screen.findByText(/Esquema de Cano Ceci actualizado/)).toBeInTheDocument();
  });

  it("unenrolls without asking: issued charges are not touched", async () => {
    const user = userEvent.setup();
    m.endEnrollments.mockResolvedValue(1);
    open();
    await user.click(await within(enrolled()).findByRole("checkbox", { name: "Cano Ceci" }));
    await user.click(screen.getByRole("button", { name: /Dar de baja/ }));
    expect(m.endEnrollments).toHaveBeenCalledWith(expect.any(String), ACT, ["s3"]);
    expect(await screen.findByText("1 alumno dado de baja")).toBeInTheDocument();
    expect(within(enrolled()).queryByText("Cano Ceci")).not.toBeInTheDocument();
  });

  it("the unenrolment toast stays at least 8 seconds", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      m.endEnrollments.mockResolvedValue(1);
      open();
      await user.click(await within(enrolled()).findByRole("checkbox", { name: "Cano Ceci" }));
      await user.click(screen.getByRole("button", { name: /Dar de baja/ }));
      expect(await screen.findByText("1 alumno dado de baja")).toBeInTheDocument();
      act(() => vi.advanceTimersByTime(7900));
      expect(screen.getByText("1 alumno dado de baja")).toBeInTheDocument();
      act(() => vi.advanceTimersByTime(200));
      expect(screen.queryByText("1 alumno dado de baja")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows the gateway's refusal and keeps the lists as they were", async () => {
    const user = userEvent.setup();
    m.enrollStudents.mockRejectedValue(new Error("La actividad está inactiva."));
    open();
    await within(enrolled()).findByText("Cano Ceci");
    await user.click(screen.getByRole("checkbox", { name: "Arce Ana" }));
    await user.click(screen.getByRole("button", { name: /Inscribir/ }));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Inscribir" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("La actividad está inactiva.");
    expect(within(enrolled()).queryByText("Arce Ana")).not.toBeInTheDocument();
  });

  it("read-only without activities:write", async () => {
    open(false);
    expect(await within(enrolled()).findByText("Cano Ceci")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Inscribir/ })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Esquema de cobro")).not.toBeInTheDocument();
    expect(within(enrolled()).getByText("Cuota fija — $900.00")).toBeInTheDocument();
  });
});

import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../scholarships/api";
import * as roster from "../../_design/students/useStudentRoster";
import { ScholarshipAssignPage } from "./ScholarshipAssignPage";

vi.mock("../../scholarships/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../scholarships/api")>();
  return { ...real, listScholarshipHolders: vi.fn(), assignScholarship: vi.fn(), unassignScholarship: vi.fn() };
});
vi.mock("../../_design/students/useStudentRoster", () => ({ useStudentRoster: vi.fn() }));
const m = vi.mocked(api);

const BECA = "b1000000-0000-0000-0000-000000000001";
const hermanos: api.Scholarship = { id: BECA, name: "HERMANOS", kind: "percent", value: 5000, targets: ["tuition"], active: true, assignedCount: 1 };
const targets: api.ScholarshipTarget[] = [{ key: "tuition", label: "Colegiatura", group: "tuition" }];

const pupils: roster.RosterStudent[] = [
  { id: "s1", lastName: "Arce", firstName: "Ana", levelId: "pri", gradeLevelId: "", groupId: "", room: "1° PRIMARIA · A" },
  { id: "s2", lastName: "Bello", firstName: "Beto", levelId: "pri", gradeLevelId: "", groupId: "", room: "" },
  { id: "s3", lastName: "Cano", firstName: "Ceci", levelId: "pri", gradeLevelId: "", groupId: "", room: "" },
];

beforeEach(() => {
  vi.mocked(roster.useStudentRoster).mockReturnValue({ status: "ok", students: pupils, catalog: { levels: [{ id: "pri", name: "Primaria" }], grades: [], groups: [] } });
  m.listScholarshipHolders.mockResolvedValue([{ studentId: "s3", startsOn: "2026-10-01", overlaps: [{ scholarshipId: "b2", name: "Excelencia" }] }]);
});
afterEach(() => vi.clearAllMocks());

function open(canWrite = true, scholarship: api.Scholarship = hermanos): void {
  render(<ScholarshipAssignPage scholarship={scholarship} targets={targets} canWrite={canWrite} onBack={() => undefined} />);
}

const holders = (): HTMLElement => screen.getByRole("region", { name: "Con beca" });

describe("ScholarshipAssignPage (T18)", () => {
  it("shows who holds it, what it is, and warns about the scholarships that do not stack", async () => {
    open();
    expect(screen.getByRole("heading", { name: "Alumnos con Beca HERMANOS" })).toBeInTheDocument();
    expect(screen.getByText("50 % · Colegiatura")).toBeInTheDocument();
    expect(await within(holders()).findByText("Cano Ceci")).toBeInTheDocument();
    expect(within(holders()).getByText("También tiene: Beca Excelencia — no se acumulan; se aplica la más favorable")).toBeInTheDocument();
    expect(m.listScholarshipHolders).toHaveBeenCalledWith(expect.any(String), BECA);
  });

  it("asks first, then gives it to the marked pupils and says how many", async () => {
    const user = userEvent.setup();
    m.assignScholarship.mockResolvedValue([
      { studentId: "s1", startsOn: "2026-10-02", overlaps: [] },
      { studentId: "s2", startsOn: "2026-10-02", overlaps: [{ scholarshipId: "b2", name: "Excelencia" }] },
    ]);
    open();
    await within(holders()).findByText("Cano Ceci");
    await user.click(screen.getByRole("checkbox", { name: "Arce Ana" }));
    await user.click(screen.getByRole("checkbox", { name: "Bello Beto" }));
    await user.click(screen.getByRole("button", { name: /Asignar beca/ }));

    const dialog = await screen.findByRole("dialog", { name: "Asignar beca" });
    expect(dialog).toHaveTextContent("¿Dar Beca HERMANOS a 2 alumnos?");
    expect(dialog).toHaveTextContent(/cargos pendientes/);
    expect(dialog).toHaveTextContent(/Lo ya pagado no cambia/);
    expect(m.assignScholarship).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole("button", { name: "Asignar" }));

    expect(m.assignScholarship).toHaveBeenCalledWith(expect.any(String), BECA, ["s1", "s2"]);
    expect(await screen.findByText("2 alumnos con Beca HERMANOS")).toBeInTheDocument();
    expect(within(holders()).getByText("Arce Ana")).toBeInTheDocument();
    expect(within(holders()).getAllByText(/También tiene: Beca Excelencia/)).toHaveLength(2);
  });

  it("cancelling the confirmation gives it to nobody", async () => {
    const user = userEvent.setup();
    open();
    await within(holders()).findByText("Cano Ceci");
    await user.click(screen.getByRole("checkbox", { name: "Arce Ana" }));
    await user.click(screen.getByRole("button", { name: /Asignar beca/ }));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Cancelar" }));
    expect(m.assignScholarship).not.toHaveBeenCalled();
    expect(screen.getByRole("checkbox", { name: "Arce Ana" })).toBeChecked();
  });

  it("takes it away without asking, and the toast stays at least 8 seconds", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      m.unassignScholarship.mockResolvedValue(1);
      open();
      await user.click(await within(holders()).findByRole("checkbox", { name: "Cano Ceci" }));
      await user.click(screen.getByRole("button", { name: /Quitar beca/ }));
      expect(m.unassignScholarship).toHaveBeenCalledWith(expect.any(String), BECA, ["s3"]);
      expect(await screen.findByText("1 alumno ya sin Beca HERMANOS")).toBeInTheDocument();
      expect(within(holders()).queryByText("Cano Ceci")).not.toBeInTheDocument();
      act(() => vi.advanceTimersByTime(7900));
      expect(screen.getByText("1 alumno ya sin Beca HERMANOS")).toBeInTheDocument();
      act(() => vi.advanceTimersByTime(200));
      expect(screen.queryByText("1 alumno ya sin Beca HERMANOS")).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows the gateway's refusal and keeps the lists as they were", async () => {
    const user = userEvent.setup();
    m.assignScholarship.mockRejectedValue(new Error("La beca está inactiva."));
    open();
    await within(holders()).findByText("Cano Ceci");
    await user.click(screen.getByRole("checkbox", { name: "Arce Ana" }));
    await user.click(screen.getByRole("button", { name: /Asignar beca/ }));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Asignar" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("La beca está inactiva.");
    expect(within(holders()).queryByText("Arce Ana")).not.toBeInTheDocument();
  });

  it("read-only without scholarships:write", async () => {
    open(false);
    expect(await within(holders()).findByText("Cano Ceci")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Asignar beca/ })).not.toBeInTheDocument();
  });

  it("an inactive scholarship cannot be given: it says so and nothing moves", async () => {
    open(true, { ...hermanos, active: false });
    expect(await within(holders()).findByText("Cano Ceci")).toBeInTheDocument();
    expect(screen.getByText(/está inactiva/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Asignar beca/ })).not.toBeInTheDocument();
  });
});

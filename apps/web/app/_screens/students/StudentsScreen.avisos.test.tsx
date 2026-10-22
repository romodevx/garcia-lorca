import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen, placementRefusal } from "./StudentsScreen";

// PLAN-import-avisos: ⚠ with the open import issues, the draft's gray pill,
// the «Con avisos» and «Borradores» tabs, and a draft's refused actions.

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
  listContacts: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
  listLabelledGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const pupil = (id: string, last: string, first: string, over: Partial<studentsApi.Student> = {}): studentsApi.Student => ({
  id, curp: `CURP${id}`, firstName: first, lastName: last, level: "Primaria", levelId: "pri", gradeGroup: "A", guardianName: "Tutor", admissionStatus: "enrolled", ...over,
});

beforeEach(() => {
  window.history.replaceState(null, "", "/alumnos");
  students.listStudents.mockResolvedValue([
    pupil("1", "Arce", "Ana", { openIssues: 2, draft: false }),
    pupil("2", "Bello", "Beto", { openIssues: 0, draft: false }),
    pupil("3", "Cano", "Ceci", { curp: "", admissionStatus: "draft", draft: true, openIssues: 1 }),
  ]);
  students.listEnrollments.mockResolvedValue([]);
  students.listContacts.mockResolvedValue([
    { id: "k1", studentId: "3", guardianUserId: "", fullName: "Mamá Cano", relationship: "mother", phone: "", email: "mama@correo.mx", isEmergency: false, hasAccount: false },
  ]);
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listLevels.mockResolvedValue([{ id: "pri", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" }]);
  config.listGradeLevels.mockResolvedValue([{ id: "pri-1", levelId: "pri", name: "1°", ordinal: 1 }]);
  config.listLabelledGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "pri-1", name: "A", capacity: 25, label: "1° PRIMARIA · A" }]);
});
afterEach(() => vi.clearAllMocks());

const names = (): string[] =>
  screen.queryAllByRole("button", { name: /^Ver ficha de / }).map((b) => (b.getAttribute("aria-label") ?? "").replace("Ver ficha de ", ""));

describe("flag OFF: the list is as before", () => {
  it("shows no ⚠, no draft tabs", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Arce Ana");
    expect(screen.queryByLabelText("2 avisos de importación")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Con avisos/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Borradores/ })).not.toBeInTheDocument();
  });
});

describe("flag ON", () => {
  it("marks a pupil with open import issues, by count, in words", async () => {
    render(<StudentsScreen onBack={() => {}} importReview />);
    await screen.findByText("Arce Ana");
    expect(screen.getByLabelText("2 avisos de importación")).toBeInTheDocument();
    expect(screen.getByLabelText("1 aviso de importación")).toBeInTheDocument();
    expect(screen.queryByLabelText("0 avisos de importación")).not.toBeInTheDocument();
  });

  it("labels a draft with the gray pill", async () => {
    render(<StudentsScreen onBack={() => {}} importReview />);
    await screen.findByText("Cano Ceci");
    expect(screen.getAllByText("Borrador · falta CURP").length).toBeGreaterThan(0);
  });

  it("filters by Con avisos and Borradores, with their counts", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} importReview />);
    await screen.findByText("Arce Ana");
    const issues = screen.getByRole("button", { name: /^Con avisos/ });
    expect(issues).toHaveTextContent("2");
    await user.click(issues);
    expect(names()).toEqual(["Arce Ana", "Cano Ceci"]);
    await user.click(screen.getByRole("button", { name: /^Borradores/ }));
    expect(names()).toEqual(["Cano Ceci"]);
  });

  // A draft cannot be enrolled, so it is not an application in progress: it
  // lives under «Borradores» only, never under «Trámite».
  it("does not count a draft under Trámite", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} importReview />);
    await screen.findByText("Cano Ceci");
    const pending = screen.getByRole("button", { name: /^Trámite/ });
    expect(pending).toHaveTextContent("0");
    await user.click(pending);
    expect(names()).toEqual([]);
  });

  it("offers a draft no enrollment and no app access, and says why", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} importReview />);
    await user.click(await screen.findByRole("button", { name: "Ver ficha de Cano Ceci" }));
    const sheet = await screen.findByRole("dialog");
    expect(await within(sheet).findByText(/Es un borrador: falta la CURP/)).toBeInTheDocument();
    expect(within(sheet).queryByRole("button", { name: "Inscribir" })).not.toBeInTheDocument();
    await within(sheet).findByText("Mamá Cano");
    expect(within(sheet).queryByRole("button", { name: /Dar acceso a la app/ })).not.toBeInTheDocument();
    expect(within(sheet).getByText("Falta la CURP del alumno para invitarle")).toBeInTheDocument();
  });

  it("still offers the enrollment to a pupil that is not a draft", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} importReview />);
    await user.click(await screen.findByRole("button", { name: "Ver ficha de Bello Beto" }));
    expect(await within(await screen.findByRole("dialog")).findByRole("button", { name: "Inscribir" })).toBeInTheDocument();
  });
});

describe("a 409 STUDENT_DRAFT that slips through", () => {
  it("is worded, not shown raw", () => {
    expect(placementRefusal(new studentsApi.StudentsError(409, "student is a draft", "STUDENT_DRAFT"))).toBe(
      "Es un borrador: falta la CURP. Captúrala en «Editar ficha» para poder inscribirlo.",
    );
  });
});

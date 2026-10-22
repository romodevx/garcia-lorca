import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as issuesApi from "../../students/importIssues";
import * as configApi from "../../config/api";
import { StudentEditScreen } from "./StudentEditScreen";

// PLAN-import-avisos: what the import changed or dropped, under its field.

vi.mock("../../students/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../students/api")>();
  return {
    StudentsError: real.StudentsError,
    getStudent: vi.fn(),
    updateStudent: vi.fn(),
    getMedical: vi.fn(),
    updateMedical: vi.fn(),
    getPhotoBlob: vi.fn(),
    uploadPhoto: vi.fn(),
    deletePhoto: vi.fn(),
    enroll: vi.fn(),
  };
});
vi.mock("../../students/importIssues", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../students/importIssues")>()),
  listImportIssues: vi.fn(),
  acceptImportIssue: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listLabelledGroups: vi.fn() }));
vi.mock("./ContactsPanel", () => ({ ContactsPanel: () => <div>contactos</div> }));
vi.mock("../../_design/students/useStudentNotes", () => ({ useStudentNotes: () => ({ notes: [] }) }));
vi.mock("../../_design/students/StudentNotesPanel", () => ({ StudentNotesPanel: () => <div>notas</div> }));

const api = vi.mocked(studentsApi);
const issues = vi.mocked(issuesApi);
const config = vi.mocked(configApi);

const EMPTY_ADDRESS = { street: "", extNumber: "", intNumber: "", neighborhood: "", postalCode: "", municipality: "", state: "" };
const student: studentsApi.Student = {
  id: "st1", curp: "LOAA190312MDFVDA01", firstName: "ADA", lastName: "LOVELACE", level: "Primaria", levelId: "lv1", gradeGroup: "4° A",
  guardianName: "", admissionStatus: "enrolled", openIssues: 2, draft: false,
  profile: { birthDate: "2019-03-12", sex: "F", email: "", phones: [], address: EMPTY_ADDRESS },
};
const draft: studentsApi.Student = { ...student, curp: "", admissionStatus: "draft", draft: true, openIssues: 1 };

const issue = (over: Partial<issuesApi.StudentImportIssue>): issuesApi.StudentImportIssue => ({
  id: "i1", studentId: "st1", field: "birth_date", code: "birth_date_from_curp", originalValue: "31/02/2019", storedValue: "2019-03-12",
  createdAt: "2026-10-21T10:00:00Z", resolution: "", ...over,
});

beforeEach(() => {
  api.getStudent.mockResolvedValue(student);
  api.getPhotoBlob.mockResolvedValue(null);
  api.updateStudent.mockImplementation(async (_b, _id, input) => ({ ...student, ...input }));
  issues.listImportIssues.mockResolvedValue([
    issue({}),
    issue({ id: "i2", field: "phone_mobile", code: "phone_invalid", originalValue: "55 12", storedValue: "" }),
  ]);
  issues.acceptImportIssue.mockImplementation(async (_b, _s, id) => issue({ id, resolution: "accepted" }));
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-07-31" }]);
  config.listLabelledGroups.mockResolvedValue([{ id: "g2a", cycleId: "c1", gradeLevelId: "gl2", name: "A", capacity: 30, label: "2° PRIMARIA · A" }]);
});
afterEach(() => vi.clearAllMocks());

function renderScreen(importReview: boolean) {
  render(<StudentEditScreen studentId="st1" userId="u" permissions={["students:read", "students:update"]} onBack={vi.fn()} today="2026-10-21" importReview={importReview} />);
  return userEvent.setup();
}

const BIRTH_WARNING = "La fecha no coincide con la CURP; se usará la de la CURP. En el Excel decía «31/02/2019»; se guardó «2019-03-12».";
const PHONE_WARNING = "Celular: Teléfono incompleto. En el Excel decía «55 12»; no se guardó.";

describe("flag OFF", () => {
  it("never asks for import issues and warns nothing", async () => {
    renderScreen(false);
    await screen.findByLabelText("Fecha de nacimiento");
    expect(issues.listImportIssues).not.toHaveBeenCalled();
    expect(screen.queryByText(/En el Excel decía/)).not.toBeInTheDocument();
  });
});

describe("flag ON", () => {
  it("warns under the affected field, tied to it, without marking it invalid", async () => {
    renderScreen(true);
    const warning = await screen.findByText(BIRTH_WARNING);
    const field = screen.getByLabelText("Fecha de nacimiento");
    expect(field.getAttribute("aria-describedby")).toContain(warning.id);
    expect(field).not.toHaveAttribute("aria-invalid");
  });

  it("warns about a dropped phone in the contact section", async () => {
    renderScreen(true);
    expect(await screen.findByText(PHONE_WARNING)).toBeInTheDocument();
  });

  it("«Está bien así» closes the issue and the warning goes", async () => {
    const user = renderScreen(true);
    await screen.findByText(BIRTH_WARNING);
    await user.click(screen.getByRole("button", { name: "Está bien así: Fecha de nacimiento" }));
    expect(issues.acceptImportIssue).toHaveBeenCalledWith(expect.any(String), "st1", "i1");
    await waitFor(() => expect(screen.queryByText(BIRTH_WARNING)).not.toBeInTheDocument());
  });

  it("brings the warning back when accepting fails", async () => {
    issues.acceptImportIssue.mockRejectedValueOnce(new studentsApi.StudentsError(500, "boom"));
    const user = renderScreen(true);
    await screen.findByText(BIRTH_WARNING);
    await user.click(screen.getByRole("button", { name: "Está bien así: Fecha de nacimiento" }));
    expect(await screen.findByText(BIRTH_WARNING)).toBeInTheDocument();
    expect(screen.getByText("No se pudo cerrar el aviso. Intenta de nuevo.")).toBeInTheDocument();
  });

  it("asks again after a save, which closes the corrected issues", async () => {
    const user = renderScreen(true);
    await screen.findByText(BIRTH_WARNING);
    issues.listImportIssues.mockResolvedValue([]);
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
    await waitFor(() => expect(issues.listImportIssues).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByText(BIRTH_WARNING)).not.toBeInTheDocument());
  });
});

describe("a draft (flag ON)", () => {
  beforeEach(() => {
    api.getStudent.mockResolvedValue(draft);
    issues.listImportIssues.mockResolvedValue([issue({ id: "ic", field: "curp", code: "curp_missing", originalValue: "", storedValue: "", intendedGroupId: "g2a" })]);
  });

  it("says the CURP is missing and offers no «Está bien así» for it", async () => {
    renderScreen(true);
    expect(await screen.findByText("Falta la CURP: sin ella no se puede inscribir")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Está bien así: CURP" })).not.toBeInTheDocument();
  });

  it("saves its other fields without a CURP", async () => {
    const user = renderScreen(true);
    await screen.findByText("Falta la CURP: sin ella no se puede inscribir");
    api.updateStudent.mockResolvedValue({ ...draft });
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
    await waitFor(() => expect(api.updateStudent).toHaveBeenCalled());
  });

  it("once its CURP is saved, offers the enrollment in the import's group", async () => {
    const user = renderScreen(true);
    const curp = await screen.findByLabelText("CURP");
    await user.type(curp, "LOAA190312MDFVDA01");
    api.updateStudent.mockResolvedValue({ ...draft, curp: "LOAA190312MDFVDA01", admissionStatus: "pending", draft: false, promotedFromDraft: true, intendedGroupId: "g2a" });
    api.enroll.mockResolvedValue({ id: "e1", studentId: "st1", cycleId: "c1", groupId: "g2a", termId: "", status: "active", enrolledAt: "2026-10-21" });

    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));

    const notice = await screen.findByRole("region", { name: "Ya no es borrador" });
    await user.click(await within(notice).findByRole("button", { name: "Inscribir en 2° PRIMARIA · A" }));
    expect(api.enroll).toHaveBeenCalledWith(expect.any(String), { studentId: "st1", groupId: "g2a" });
    expect(await within(notice).findByText("Inscrito en 2° PRIMARIA · A.")).toBeInTheDocument();
  });
});

import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { StudentEditScreen } from "./StudentEditScreen";
import type { FamiliesAccess } from "../../families/model";

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
  };
});
vi.mock("./ContactsPanel", () => ({
  ContactsPanel: ({ familyFull, accessReach }: { familyFull?: boolean; accessReach?: string }) => (
    <div>
      <div>{`contactos:${familyFull === true ? "llena" : "con-lugar"}`}</div>
      <div>{`alcance:${accessReach ?? "ninguno"}`}</div>
    </div>
  ),
}));
vi.mock("../../_design/students/useStudentNotes", () => ({ useStudentNotes: () => ({ notes: [] }) }));
const kid = (id: string, firstName: string) => ({ id, firstName, lastName: "LOVELACE", maternalLastName: "", level: "", gradeGroup: "", admissionStatus: "enrolled", draft: false });
vi.mock("../families/FamilyCard", () => ({
  FamilyCard: ({ familyId, onFamilyRead }: { familyId?: string; onFamilyRead?: (f: object) => void }) => (
    <button type="button" onClick={() => onFamilyRead?.({ id: familyId ?? "", accountCount: 2, studentIds: ["st1", "st2"], members: [kid("st1", "ADA"), kid("st2", "BYRON")] })}>{`familia:${familyId ?? "ninguna"}`}</button>
  ),
}));
vi.mock("../../_design/students/StudentNotesPanel", () => ({ StudentNotesPanel: () => <div>notas</div> }));

const api = vi.mocked(studentsApi);

const student: studentsApi.Student = {
  id: "st1",
  curp: "LOAA190312MDFVDA01",
  firstName: "ADA",
  lastName: "LOVELACE",
  level: "Primaria",
  levelId: "lv1",
  gradeGroup: "4° A",
  guardianName: "",
  admissionStatus: "enrolled",
  bankReference: "231076",
  familyId: "f1",
  profile: { birthDate: "", sex: "F", email: "", phones: [], address: { street: "", extNumber: "", intNumber: "", neighborhood: "", postalCode: "", municipality: "", state: "" } },
};

beforeEach(() => {
  api.getStudent.mockResolvedValue(student);
  api.getPhotoBlob.mockResolvedValue(null);
  api.updateStudent.mockImplementation(async (_b, _id, input) => ({ ...student, ...input }));
});
afterEach(() => vi.clearAllMocks());

const OWNER: FamiliesAccess = { canReadGuardians: true, canWriteGuardians: true, canUpdateStudents: true };

describe("StudentEditScreen — Familia (ADR 0041)", () => {
  it("module_families off: no Familia card", async () => {
    render(<StudentEditScreen studentId="st1" userId="u" permissions={["students:read", "students:update"]} onBack={vi.fn()} today="2026-10-06" />);
    await screen.findByLabelText("Referencia BBVA");
    expect(screen.queryByRole("region", { name: "Familia" })).toBeNull();
    expect(screen.queryByText(/^familia:/)).toBeNull();
  });

  it("on: the card shows the pupil's family, and the index links to it", async () => {
    render(<StudentEditScreen studentId="st1" userId="u" permissions={["students:read", "students:update"]} onBack={vi.fn()} today="2026-10-06" families={OWNER} onOpenFamily={vi.fn()} onOpenStudent={vi.fn()} />);
    const card = await screen.findByRole("region", { name: "Familia" });
    expect(card).toHaveTextContent("familia:f1");
    expect(screen.getByRole("button", { name: "Familia" })).toBeInTheDocument();
  });
  it("a family with two accounts reaches the contacts (QA-3)", async () => {
    render(<StudentEditScreen studentId="st1" userId="u" permissions={["students:read", "students:update"]} onBack={vi.fn()} today="2026-10-06" families={OWNER} onOpenFamily={vi.fn()} onOpenStudent={vi.fn()} />);
    expect(await screen.findByText("contactos:con-lugar")).toBeInTheDocument();
    (await screen.findByRole("button", { name: "familia:f1" })).click();
    expect(await screen.findByText("contactos:llena")).toBeInTheDocument();
  });

  it("the family the card read tells the contacts which children an account sees (M2), with no request of its own", async () => {
    render(<StudentEditScreen studentId="st1" userId="u" permissions={["students:read", "students:update"]} onBack={vi.fn()} today="2026-10-06" families={OWNER} onOpenFamily={vi.fn()} onOpenStudent={vi.fn()} />);
    // Until (or unless) the card reads the family, the line fails closed.
    expect(await screen.findByText("alcance:Verá a todos los alumnos de su familia")).toBeInTheDocument();
    (await screen.findByRole("button", { name: "familia:f1" })).click();
    expect(await screen.findByText("alcance:Verá a ADA y BYRON")).toBeInTheDocument();
  });

  it("a pupil without a family: the contacts get no line", async () => {
    api.getStudent.mockResolvedValue({ ...student, familyId: undefined });
    render(<StudentEditScreen studentId="st1" userId="u" permissions={["students:read", "students:update"]} onBack={vi.fn()} today="2026-10-06" families={OWNER} onOpenFamily={vi.fn()} onOpenStudent={vi.fn()} />);
    expect(await screen.findByText("alcance:ninguno")).toBeInTheDocument();
  });

  it("module_families off: the contacts get no line", async () => {
    render(<StudentEditScreen studentId="st1" userId="u" permissions={["students:read", "students:update"]} onBack={vi.fn()} today="2026-10-06" />);
    expect(await screen.findByText("alcance:ninguno")).toBeInTheDocument();
  });
});

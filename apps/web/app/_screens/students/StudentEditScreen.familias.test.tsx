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
vi.mock("./ContactsPanel", () => ({ ContactsPanel: () => <div>contactos</div> }));
vi.mock("../../_design/students/useStudentNotes", () => ({ useStudentNotes: () => ({ notes: [] }) }));
vi.mock("../families/FamilyCard", () => ({
  FamilyCard: ({ familyId }: { familyId?: string }) => <div>{`familia:${familyId ?? "ninguna"}`}</div>,
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
});

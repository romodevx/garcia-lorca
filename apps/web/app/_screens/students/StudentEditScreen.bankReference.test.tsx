import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { StudentEditScreen } from "./StudentEditScreen";

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
  profile: { birthDate: "", sex: "F", email: "", phones: [], address: { street: "", extNumber: "", intNumber: "", neighborhood: "", postalCode: "", municipality: "", state: "" } },
};

beforeEach(() => {
  api.getStudent.mockResolvedValue(student);
  api.getPhotoBlob.mockResolvedValue(null);
  api.updateStudent.mockImplementation(async (_b, _id, input) => ({ ...student, ...input }));
});
afterEach(() => vi.clearAllMocks());

function renderScreen() {
  render(<StudentEditScreen studentId="st1" userId="u" permissions={["students:read", "students:update"]} onBack={vi.fn()} today="2026-10-06" />);
  return userEvent.setup();
}

describe("StudentEditScreen — Referencia BBVA (ADR 0033)", () => {
  it("shows the stored reference", async () => {
    renderScreen();
    expect(await screen.findByLabelText("Referencia BBVA")).toHaveValue("231076");
  });

  it("leaves it out of the save when it did not change, so the stored one is kept", async () => {
    const user = renderScreen();
    await screen.findByLabelText("Referencia BBVA");
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));

    await waitFor(() => expect(api.updateStudent).toHaveBeenCalled());
    expect(api.updateStudent.mock.calls[0]?.[2]).not.toHaveProperty("bankReference");
  });

  it("sends the new one when it changed, and \"\" when cleared", async () => {
    const user = renderScreen();
    const field = await screen.findByLabelText("Referencia BBVA");
    await user.clear(field);
    await user.type(field, "998877");
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
    await waitFor(() => expect(api.updateStudent.mock.calls[0]?.[2]).toMatchObject({ bankReference: "998877" }));

    await user.clear(field);
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
    await waitFor(() => expect(api.updateStudent.mock.calls[1]?.[2]).toMatchObject({ bankReference: "" }));
  });

  it("says on blur that it takes digits only, up to 20, and refuses to save", async () => {
    const user = renderScreen();
    const field = await screen.findByLabelText("Referencia BBVA");
    await user.clear(field);
    await user.type(field, "S/R");
    expect(screen.queryByText("Sólo dígitos, hasta 20.")).not.toBeInTheDocument();
    await user.tab();
    expect(screen.getByText("Sólo dígitos, hasta 20.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
    expect(api.updateStudent).not.toHaveBeenCalled();
  });

  it("refuses more than 20 digits", async () => {
    const user = renderScreen();
    const field = await screen.findByLabelText("Referencia BBVA");
    await user.clear(field);
    await user.type(field, "123456789012345678901");
    await user.tab();
    expect(screen.getByText("Sólo dígitos, hasta 20.")).toBeInTheDocument();
  });
});

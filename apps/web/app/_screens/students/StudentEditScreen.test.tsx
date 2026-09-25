import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as resize from "../../students/photoResize";
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
vi.mock("../../students/photoResize", () => ({ resizeToJpeg: vi.fn() }));
vi.mock("./ContactsPanel", () => ({ ContactsPanel: () => <div>contactos</div> }));
vi.mock("./NotesPanel", () => ({
  NotesPanel: (p: { userId: string; canWrite: boolean; canVoidAny: boolean }) => (
    <div>{`notas de ${p.userId} write=${String(p.canWrite)} voidAny=${String(p.canVoidAny)}`}</div>
  ),
}));

const api = vi.mocked(studentsApi);
const shrink = vi.mocked(resize);

const TODAY = "2026-09-24";

const student: studentsApi.Student = {
  id: "st1",
  curp: "LOAA190312MDFVDA01",
  firstName: "ADA",
  lastName: "LOVELACE BYRON",
  level: "Primaria",
  levelId: "lv1",
  gradeGroup: "4° A",
  guardianName: "Marisol Ríos",
  admissionStatus: "enrolled",
  enrollmentNumber: "2026-00042",
  profile: {
    birthDate: "2019-03-12",
    sex: "F",
    email: "ada@correo.com",
    phones: [{ kind: "casa", number: "5512345678" }],
    address: {
      street: "Juárez",
      extNumber: "12",
      intNumber: "",
      neighborhood: "Centro",
      postalCode: "01000",
      municipality: "Cuauhtémoc",
      state: "Ciudad de México",
    },
  },
};

function renderScreen(permissions: readonly string[] = ["students:read", "students:update"]) {
  const onBack = vi.fn();
  render(<StudentEditScreen studentId="st1" userId="u-me" permissions={permissions} onBack={onBack} today={TODAY} />);
  return { onBack };
}

beforeEach(() => {
  api.getStudent.mockResolvedValue(student);
  api.getPhotoBlob.mockResolvedValue(null);
  api.getMedical.mockResolvedValue({ bloodType: "O+", allergies: "nuez", conditions: "" });
  api.updateStudent.mockImplementation(async (_b, _id, input) => ({ ...student, ...input }));
  api.updateMedical.mockImplementation(async (_b, _id, r) => r);
  api.uploadPhoto.mockResolvedValue(undefined);
  api.deletePhoto.mockResolvedValue(undefined);
  globalThis.URL.createObjectURL = vi.fn(() => "blob:preview");
  globalThis.URL.revokeObjectURL = vi.fn();
});
afterEach(() => vi.clearAllMocks());

describe("StudentEditScreen", () => {
  it("prefills the whole file, with level and group read-only", async () => {
    renderScreen();

    expect(await screen.findByLabelText("Nombre(s)")).toHaveValue("ADA");
    expect(screen.getByLabelText("Apellidos")).toHaveValue("LOVELACE BYRON");
    expect(screen.getByLabelText("CURP")).toHaveValue("LOAA190312MDFVDA01");
    // Task 13: the matrícula is shown, never edited.
    expect(screen.getByText("2026-00042")).toBeInTheDocument();
    expect(screen.getByLabelText("Fecha de nacimiento")).toHaveValue("2019-03-12");
    expect(screen.getByLabelText("Sexo")).toHaveValue("F");
    expect(screen.getByLabelText("Correo del alumno")).toHaveValue("ada@correo.com");
    expect(screen.getByLabelText("Teléfono 1")).toHaveValue("5512345678");
    expect(screen.getByLabelText("Código postal")).toHaveValue("01000");
    expect(screen.getByLabelText("Estado")).toHaveValue("Ciudad de México");
    expect(screen.getByText("Primaria · 4° A")).toBeInTheDocument();
    expect(screen.queryByLabelText("Nivel")).not.toBeInTheDocument();
  });

  it("saves names, CURP and profile in one request", async () => {
    const user = userEvent.setup();
    renderScreen();
    const street = await screen.findByLabelText("Calle");
    await user.clear(street);
    await user.type(street, "Madero");
    await user.click(screen.getByRole("button", { name: "Agregar teléfono" }));
    await user.selectOptions(screen.getByLabelText("Tipo de teléfono 2"), "celular");
    await user.type(screen.getByLabelText("Teléfono 2"), "55 9876 5432");
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));

    await waitFor(() => expect(api.updateStudent).toHaveBeenCalled());
    const [, id, input] = api.updateStudent.mock.calls[0] ?? [];
    expect(id).toBe("st1");
    expect(input).toEqual({
      firstName: "ADA",
      lastName: "LOVELACE BYRON",
      curp: "LOAA190312MDFVDA01",
      profile: {
        ...student.profile,
        phones: [
          { kind: "casa", number: "5512345678" },
          { kind: "celular", number: "5598765432" },
        ],
        address: { ...student.profile?.address, street: "Madero" },
      },
    });
    expect(await screen.findByText("Cambios guardados.")).toBeInTheDocument();
  });

  it("allows at most three phones", async () => {
    const user = userEvent.setup();
    renderScreen();
    await user.click(await screen.findByRole("button", { name: "Agregar teléfono" }));
    await user.click(screen.getByRole("button", { name: "Agregar teléfono" }));
    expect(screen.getByLabelText("Teléfono 3")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Agregar teléfono" })).not.toBeInTheDocument();
  });

  it("marks a future birth date when the field is left, and does not save it", async () => {
    const user = userEvent.setup();
    renderScreen();
    const born = await screen.findByLabelText("Fecha de nacimiento");
    fireEvent.change(born, { target: { value: "2026-12-01" } });
    fireEvent.blur(born);

    expect(await screen.findByText("La fecha de nacimiento no puede ser futura.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
    expect(api.updateStudent).not.toHaveBeenCalled();
  });

  it("on save, marks a bad postal code and takes the focus there", async () => {
    const user = userEvent.setup();
    renderScreen();
    const cp = await screen.findByLabelText("Código postal");
    await user.clear(cp);
    await user.type(cp, "0100");
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));

    expect(api.updateStudent).not.toHaveBeenCalled();
    expect(cp).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => expect(document.activeElement).toBe(cp));
  });

  it("refuses to save without the pupil's names", async () => {
    const user = userEvent.setup();
    renderScreen();
    await user.clear(await screen.findByLabelText("Nombre(s)"));
    await user.click(screen.getByRole("button", { name: "Guardar ficha" }));

    expect(api.updateStudent).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Nombre(s)")).toHaveAttribute("aria-invalid", "true");
  });

  it("says what the server refused", async () => {
    const user = userEvent.setup();
    api.updateStudent.mockRejectedValue(new studentsApi.StudentsError(409, "curp taken"));
    renderScreen();
    await user.click(await screen.findByRole("button", { name: "Guardar ficha" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Ya hay otro alumno con ese CURP.");
  });

  it("saves with Cmd/Ctrl+Enter from any field", async () => {
    renderScreen();
    const street = await screen.findByLabelText("Calle");
    fireEvent.keyDown(street, { key: "Enter", metaKey: true });
    await waitFor(() => expect(api.updateStudent).toHaveBeenCalled());
  });

  describe("medical data", () => {
    it("is not shown, nor asked for, without medical:read", async () => {
      renderScreen();
      await screen.findByLabelText("Nombre(s)");
      expect(screen.queryByText("Datos médicos")).not.toBeInTheDocument();
      expect(api.getMedical).not.toHaveBeenCalled();
    });

    // Typed into before it arrived, the record would overwrite what was typed.
    it("offers no field until the record has arrived", async () => {
      api.getMedical.mockReturnValue(new Promise(() => undefined));
      renderScreen(["students:read", "students:update", "medical:read", "medical:write"]);
      expect(await screen.findByText("Cargando datos médicos…")).toBeInTheDocument();
      expect(screen.queryByLabelText("Alergias")).not.toBeInTheDocument();
    });

    it("is read-only with medical:read alone", async () => {
      renderScreen(["students:read", "students:update", "medical:read"]);
      expect(await screen.findByLabelText("Alergias")).toHaveValue("nuez");
      expect(screen.getByLabelText("Alergias")).toBeDisabled();
      expect(screen.getByLabelText("Tipo de sangre")).toBeDisabled();
    });

    it("is saved with medical:write", async () => {
      const user = userEvent.setup();
      renderScreen(["students:read", "students:update", "medical:read", "medical:write"]);
      const conditions = await screen.findByLabelText("Padecimientos");
      await user.type(conditions, "asma");
      await user.click(screen.getByRole("button", { name: "Guardar ficha" }));

      await waitFor(() => expect(api.updateMedical).toHaveBeenCalled());
      expect(api.updateMedical.mock.calls[0]?.slice(1)).toEqual([
        "st1",
        { bloodType: "O+", allergies: "nuez", conditions: "asma" },
      ]);
    });

    it("is not re-sent when nothing in it changed", async () => {
      const user = userEvent.setup();
      renderScreen(["students:read", "students:update", "medical:read", "medical:write"]);
      await screen.findByLabelText("Padecimientos");
      await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
      await waitFor(() => expect(api.updateStudent).toHaveBeenCalled());
      expect(api.updateMedical).not.toHaveBeenCalled();
    });
  });

  describe("photo", () => {
    it("uploads a chosen file, shrunk, when the file is saved", async () => {
      const user = userEvent.setup();
      const small = new Blob(["jpeg"], { type: "image/jpeg" });
      shrink.resizeToJpeg.mockResolvedValue(small);
      renderScreen();
      const file = new File(["png"], "foto.png", { type: "image/png" });
      await user.upload(await screen.findByLabelText("Subir foto"), file);

      expect(await screen.findByRole("img", { name: "Foto de ADA LOVELACE BYRON" })).toBeInTheDocument();
      expect(api.uploadPhoto).not.toHaveBeenCalled();
      await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
      await waitFor(() => expect(api.uploadPhoto).toHaveBeenCalledWith(expect.any(String), "st1", small));
    });

    it("refuses a file that is not a JPEG or PNG", async () => {
      renderScreen();
      const input = await screen.findByLabelText("Subir foto");
      fireEvent.change(input, { target: { files: [new File(["x"], "a.gif", { type: "image/gif" })] } });
      expect(await screen.findByText("La foto tiene que ser JPG o PNG.")).toBeInTheDocument();
      expect(shrink.resizeToJpeg).not.toHaveBeenCalled();
    });

    it("removes the photo when the file is saved", async () => {
      const user = userEvent.setup();
      api.getPhotoBlob.mockResolvedValue(new Blob(["x"], { type: "image/jpeg" }));
      renderScreen();
      await user.click(await screen.findByRole("button", { name: "Quitar foto" }));
      await user.click(screen.getByRole("button", { name: "Guardar ficha" }));
      await waitFor(() => expect(api.deletePhoto).toHaveBeenCalledWith(expect.any(String), "st1"));
    });
  });

  // Task 6: the file carries its notes, under their own permissions.
  it("shows the pupil's notes with what the caller may do", async () => {
    renderScreen(["students:read", "students:update", "students:notes_write"]);
    expect(await screen.findByRole("heading", { name: "Notas" })).toBeInTheDocument();
    expect(screen.getByText("notas de u-me write=true voidAny=false")).toBeInTheDocument();
  });

  it("goes back to the list", async () => {
    const user = userEvent.setup();
    const { onBack } = renderScreen();
    await user.click(await screen.findByRole("button", { name: "Volver a alumnos" }));
    expect(onBack).toHaveBeenCalled();
  });
});

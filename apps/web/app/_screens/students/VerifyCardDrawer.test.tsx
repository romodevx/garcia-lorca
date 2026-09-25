import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { StudentsError } from "../../students/api";
import { VerifyCardDrawer } from "./VerifyCardDrawer";

const api = vi.hoisted(() => ({ verifyCard: vi.fn(), getPhotoBlob: vi.fn() }));
vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../students/api")>()),
  verifyCard: api.verifyCard,
  getPhotoBlob: api.getPhotoBlob,
}));

const zoe = { valid: true, studentId: "st1", name: "Zoe Villalobos", level: "Primaria", gradeGroup: "1° A", enrollmentNumber: "2026-00042", admissionStatus: "enrolled" };

beforeEach(() => {
  api.verifyCard.mockReset();
  api.getPhotoBlob.mockReset().mockResolvedValue(null);
});

describe("VerifyCardDrawer (tarea 13)", () => {
  it("reads a scanned code on Enter and says whose card it is and that it counts", async () => {
    api.verifyCard.mockResolvedValue(zoe);
    const user = userEvent.setup();
    render(<VerifyCardDrawer onClose={() => {}} />);

    const field = screen.getByLabelText(/Código de la credencial/);
    expect(field).toHaveFocus();
    await user.type(field, "GLCARD:AQAAAAAAAAAAAAAAAAAAAA{Enter}");

    expect(await screen.findByText("Zoe Villalobos")).toBeInTheDocument();
    expect(screen.getByText("Vigente")).toBeInTheDocument();
    expect(screen.getByText(/2026-00042/)).toBeInTheDocument();
    expect(api.verifyCard).toHaveBeenCalledWith(expect.any(String), "GLCARD:AQAAAAAAAAAAAAAAAAAAAA");
    // Ready for the next pupil in line: the field is empty and focused.
    expect(field).toHaveValue("");
    expect(field).toHaveFocus();
  });

  it("says a withdrawn pupil's card no longer counts", async () => {
    api.verifyCard.mockResolvedValue({ ...zoe, valid: false, admissionStatus: "withdrawn" });
    const user = userEvent.setup();
    render(<VerifyCardDrawer onClose={() => {}} />);

    await user.type(screen.getByLabelText(/Código de la credencial/), "GLCARD:AQAAAAAAAAAAAAAAAAAAAA{Enter}");
    expect(await screen.findByText("No vigente")).toBeInTheDocument();
  });

  it("says plainly when the code is unknown, or is not a card's", async () => {
    const user = userEvent.setup();
    render(<VerifyCardDrawer onClose={() => {}} />);
    const field = screen.getByLabelText(/Código de la credencial/);

    api.verifyCard.mockRejectedValueOnce(new StudentsError(404, "not found"));
    await user.type(field, "GLCARD:ZZZZZZZZZZZZZZZZZZZZZZ{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent("Esa credencial no está registrada.");

    api.verifyCard.mockRejectedValueOnce(new StudentsError(400, "not a student card code"));
    await user.type(field, "hola{Enter}");
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("Ese código no es de una credencial."));
  });

  it("asks for a code rather than sending nothing", async () => {
    const user = userEvent.setup();
    render(<VerifyCardDrawer onClose={() => {}} />);
    await user.click(screen.getByRole("button", { name: "Verificar" }));

    expect(api.verifyCard).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/Código de la credencial/)).toHaveFocus();
  });

  // Review of task 13: the staff compare the photo with the face in front of
  // them, so one pupil's photo must never stay under the next pupil's name.
  it("never shows the previous pupil's photo under the next one's name", async () => {
    const createObjectURL = vi.fn(() => "blob:photo-of-zoe");
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() }));
    api.verifyCard.mockResolvedValueOnce(zoe).mockResolvedValueOnce({ ...zoe, studentId: "st2", name: "Iker Reyes" });
    api.getPhotoBlob.mockResolvedValueOnce(new Blob(["jpg"])).mockResolvedValueOnce(null);
    const user = userEvent.setup();
    render(<VerifyCardDrawer onClose={() => {}} />);
    const field = screen.getByLabelText(/Código de la credencial/);

    await user.type(field, "GLCARD:AQAAAAAAAAAAAAAAAAAAAA{Enter}");
    expect(await screen.findByAltText("Foto de Zoe Villalobos")).toBeInTheDocument();

    await user.type(field, "GLCARD:AgAAAAAAAAAAAAAAAAAAAA{Enter}");
    expect(await screen.findByText("Iker Reyes")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("img")).toBeNull());
    expect(screen.getByText("IR")).toBeInTheDocument();
  });
});

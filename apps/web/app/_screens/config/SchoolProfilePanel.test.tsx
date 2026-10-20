import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SchoolProfilePanel } from "./SchoolProfilePanel";

const api = vi.hoisted(() => ({
  getSchoolProfile: vi.fn(),
  saveSchoolProfile: vi.fn(),
}));
vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../config/api")>()),
  getSchoolProfile: api.getSchoolProfile,
  saveSchoolProfile: api.saveSchoolProfile,
}));

const EMPTY = { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "", clabe: "", clabeBank: "" };

beforeEach(() => {
  api.getSchoolProfile.mockReset();
  api.saveSchoolProfile.mockReset();
});

describe("SchoolProfilePanel (tarea 12)", () => {
  it("says the details are missing, and captures them", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: EMPTY, found: false });
    api.saveSchoolProfile.mockImplementation(async (_: string, p: typeof EMPTY) => ({ ...p, rfc: p.rfc.toUpperCase() }));
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);

    expect(await screen.findByText(/Aún no hay datos del colegio/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Capturar datos" }));
    await user.type(screen.getByLabelText(/Nombre del colegio/), "Colegio García Lorca");
    await user.type(screen.getByLabelText(/^RFC/), "EGL150101AB1");
    await user.click(screen.getByRole("button", { name: "Guardar datos" }));

    await waitFor(() => expect(api.saveSchoolProfile).toHaveBeenCalledTimes(1));
    expect(api.saveSchoolProfile.mock.calls[0]?.[1]).toMatchObject({ name: "COLEGIO GARCÍA LORCA", rfc: "EGL150101AB1" });
    expect(await screen.findByText("COLEGIO GARCÍA LORCA")).toBeInTheDocument();
  });

  it("does not save without a name, and points at it", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: EMPTY, found: false });
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);

    await user.click(await screen.findByRole("button", { name: "Capturar datos" }));
    await user.click(screen.getByRole("button", { name: "Guardar datos" }));

    expect(api.saveSchoolProfile).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByLabelText(/Nombre del colegio/)).toHaveFocus());
  });

  it("checks the RFC's shape when leaving the field, and will not save it wrong", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: EMPTY, found: false });
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);

    await user.click(await screen.findByRole("button", { name: "Capturar datos" }));
    await user.type(screen.getByLabelText(/Nombre del colegio/), "Colegio");
    await user.type(screen.getByLabelText(/^RFC/), "EGL-15");
    await user.tab();

    expect(await screen.findByText(/RFC no tiene la forma del SAT/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Guardar datos" }));
    expect(api.saveSchoolProfile).not.toHaveBeenCalled();
  });

  it("shows the saved details, and edits them", async () => {
    api.getSchoolProfile.mockResolvedValue({
      profile: { ...EMPTY, name: "COLEGIO GARCÍA LORCA", rfc: "EGL150101AB1", phone: "5512345678" },
      found: true,
    });
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);

    expect(await screen.findByText("COLEGIO GARCÍA LORCA")).toBeInTheDocument();
    expect(screen.getByText(/EGL150101AB1/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Editar datos" }));
    expect(screen.getByLabelText(/Nombre del colegio/)).toHaveValue("COLEGIO GARCÍA LORCA");
  });

  // The CLABE the family app shows for transfers: one wrong digit sends the
  // money elsewhere, so the check digit is verified before saving.
  it("captures the CLABE without spaces, and shows its bank", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: { ...EMPTY, name: "COLEGIO GARCÍA LORCA" }, found: true });
    api.saveSchoolProfile.mockImplementation(async (_: string, p: typeof EMPTY) => ({ ...p, clabeBank: "BBVA México" }));
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);

    await user.click(await screen.findByRole("button", { name: "Editar datos" }));
    await user.type(screen.getByRole("textbox", { name: /^Cuenta CLABE/ }), "0121 8000 1234 5678 99");
    await user.click(screen.getByRole("button", { name: "Guardar datos" }));

    await waitFor(() => expect(api.saveSchoolProfile).toHaveBeenCalledTimes(1));
    expect(api.saveSchoolProfile.mock.calls[0]?.[1]).toMatchObject({ clabe: "012180001234567899" });
    expect(await screen.findByText(/CLABE BBVA México 012180001234567899/)).toBeInTheDocument();
  });

  it("refuses a CLABE whose check digit is wrong", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: { ...EMPTY, name: "COLEGIO GARCÍA LORCA" }, found: true });
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);

    await user.click(await screen.findByRole("button", { name: "Editar datos" }));
    await user.type(screen.getByRole("textbox", { name: /^Cuenta CLABE/ }), "012180001234567890");
    await user.tab();

    expect(await screen.findByText(/La CLABE no es válida/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Guardar datos" }));
    expect(api.saveSchoolProfile).not.toHaveBeenCalled();
  });
});

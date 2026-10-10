import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ConfigError } from "../../config/api";
import { SchoolProfilePanel } from "./SchoolProfilePanel";

/*
 * Characterization tests (2026-10-09) for "Datos del colegio", frozen before the
 * Configuración académica redesign (behavior.md §2: S2.2, S2.4 summary, S2.5,
 * A5, A6 CCT/phone).
 */

const api = vi.hoisted(() => ({
  getSchoolProfile: vi.fn(),
  saveSchoolProfile: vi.fn(),
}));
vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../config/api")>()),
  getSchoolProfile: api.getSchoolProfile,
  saveSchoolProfile: api.saveSchoolProfile,
}));

const EMPTY = { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" };
const NAMED = { ...EMPTY, name: "COLEGIO GARCÍA LORCA" };

beforeEach(() => {
  api.getSchoolProfile.mockReset();
  api.saveSchoolProfile.mockReset();
});

async function openDrawer(user: ReturnType<typeof userEvent.setup>, button: string): Promise<void> {
  await user.click(await screen.findByRole("button", { name: button }));
  await screen.findByRole("dialog", { name: "Datos del colegio" });
}

describe("SchoolProfilePanel — characterization", () => {
  it("says it could not read the details, with no button to capture them", async () => {
    api.getSchoolProfile.mockRejectedValue(new Error("down"));
    render(<SchoolProfilePanel />);

    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudieron leer los datos del colegio.");
    expect(screen.queryByRole("button", { name: /Capturar datos|Editar datos/ })).toBeNull();
  });

  it("a named school with nothing else says what is missing", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: NAMED, found: true });
    render(<SchoolProfilePanel />);

    expect(await screen.findByText("COLEGIO GARCÍA LORCA")).toBeInTheDocument();
    expect(screen.getByText("Sin RFC, CCT, teléfono ni dirección")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Editar datos" })).toBeInTheDocument();
  });

  it("the summary lists only what is filled, in receipt order: RFC · CCT · phone · address", async () => {
    api.getSchoolProfile.mockResolvedValue({
      profile: { ...NAMED, rfc: "EGL150101AB1", phone: "5512345678", address: "CALLE 1" },
      found: true,
    });
    render(<SchoolProfilePanel />);

    expect(await screen.findByText("RFC EGL150101AB1 · 5512345678 · CALLE 1")).toBeInTheDocument();
  });

  it("checks the CCT and phone shapes when leaving the field", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: NAMED, found: true });
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);
    await openDrawer(user, "Editar datos");

    await user.type(screen.getByLabelText(/^CCT/), "123");
    await user.tab();
    expect(await screen.findByText("La CCT no tiene la forma de la SEP (10 caracteres, p. ej. 09PPR1234X).")).toBeInTheDocument();

    await user.type(screen.getByLabelText(/^Teléfono/), "55 12");
    await user.tab();
    expect(await screen.findByText("El teléfono lleva 10 dígitos.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Guardar datos" }));
    expect(api.saveSchoolProfile).not.toHaveBeenCalled();
  });

  it("a 400 on save says 'Revisa los datos: {detail}' inside the drawer", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: NAMED, found: true });
    api.saveSchoolProfile.mockRejectedValue(new ConfigError(400, "rfc inválido"));
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);
    await openDrawer(user, "Editar datos");

    await user.click(screen.getByRole("button", { name: "Guardar datos" }));

    expect(await screen.findByText("Revisa los datos: rfc inválido")).toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: "Datos del colegio" })).toBeInTheDocument();
  });

  it("any other save failure says the details could not be saved", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: NAMED, found: true });
    api.saveSchoolProfile.mockRejectedValue(new ConfigError(500, ""));
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);
    await openDrawer(user, "Editar datos");

    await user.click(screen.getByRole("button", { name: "Guardar datos" }));

    expect(await screen.findByText("No se pudieron guardar los datos.")).toBeInTheDocument();
  });

  it("Cancelar closes without saving and keeps the old details", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: NAMED, found: true });
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);
    await openDrawer(user, "Editar datos");

    await user.clear(screen.getByLabelText(/^Nombre del colegio/));
    await user.type(screen.getByLabelText(/^Nombre del colegio/), "otro");
    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(api.saveSchoolProfile).not.toHaveBeenCalled();
    expect(screen.getByText("COLEGIO GARCÍA LORCA")).toBeInTheDocument();
  });

  it("Escape closes without saving", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: EMPTY, found: false });
    const user = userEvent.setup();
    render(<SchoolProfilePanel />);
    await openDrawer(user, "Capturar datos");

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(api.saveSchoolProfile).not.toHaveBeenCalled();
  });
});

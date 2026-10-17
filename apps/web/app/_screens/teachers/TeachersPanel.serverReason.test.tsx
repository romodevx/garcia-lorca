import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ConfigError } from "../../config/api";
import { TeachersPanel } from "./TeachersPanel";

async function fillAndSave(user: ReturnType<typeof userEvent.setup>, email: string): Promise<void> {
  await user.type(screen.getByLabelText(/^Nombre/), "Ana");
  await user.type(screen.getByLabelText(/^Apellidos/), "Ruiz");
  await user.type(screen.getByLabelText(/^Correo/), email);
  await user.click(screen.getByRole("button", { name: /Crear profesor/ }));
}

// QA 2026-10-15, ISSUE-012: the gateway answered 409 "a teacher with this
// email already exists" and the panel said "Intenta de nuevo" — a retry that
// can never work.
describe("TeachersPanel — the server's reason (ISSUE-012)", () => {
  it("says the email is taken when the server answers 409", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockRejectedValue(new ConfigError(409, "a teacher with this email already exists"));
    render(<TeachersPanel teachers={[]} onCreate={onCreate} />);

    await fillAndSave(user, "ana@colegio.mx");

    expect(await screen.findByText("Ya existe un docente con ese correo.")).toBeInTheDocument();
    expect(screen.queryByText(/Intenta de nuevo/)).toBeNull();
  });

  it("clears the previous error once the next teacher is created", async () => {
    const user = userEvent.setup();
    const onCreate = vi
      .fn()
      .mockRejectedValueOnce(new ConfigError(409, "a teacher with this email already exists"))
      .mockResolvedValueOnce(undefined);
    render(<TeachersPanel teachers={[]} onCreate={onCreate} />);

    await fillAndSave(user, "ana@colegio.mx");
    expect(await screen.findByText("Ya existe un docente con ese correo.")).toBeInTheDocument();

    await user.clear(screen.getByLabelText(/^Correo/));
    await user.type(screen.getByLabelText(/^Correo/), "ana.ruiz@colegio.mx");
    await user.click(screen.getByRole("button", { name: /Crear profesor/ }));

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByText("Ya existe un docente con ese correo.")).toBeNull());
  });

  it("blames the connection only when the server never answered", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockRejectedValue(new TypeError("Failed to fetch"));
    render(<TeachersPanel teachers={[]} onCreate={onCreate} />);

    await fillAndSave(user, "ana@colegio.mx");

    expect(await screen.findByText("No se pudo crear al profesor. Revisa tu conexión e intenta de nuevo.")).toBeInTheDocument();
  });
});

describe("TeachersPanel — Crear profesor while saving", () => {
  it("is busy, not disabled", async () => {
    const user = userEvent.setup();
    render(<TeachersPanel teachers={[]} onCreate={vi.fn(() => new Promise<void>(() => {}))} />);

    await fillAndSave(user, "ana@colegio.mx");

    const busy = screen.getByRole("button", { name: /Guardando/ });
    expect(busy).toBeEnabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
  });
});

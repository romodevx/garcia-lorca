import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ConfigError } from "../../config/api";
import type { Teacher, TeacherAccessResult } from "../../teachers/api";
import { TeachersPanel } from "./TeachersPanel";

const ada: Teacher = { id: "t1", firstName: "Ada", lastName: "Lovelace", email: "ada@escuela.mx", userId: "" };
const alan: Teacher = { id: "t2", firstName: "Alan", lastName: "Turing", email: "alan@escuela.mx", userId: "u2" };

function outcome(over: Partial<TeacherAccessResult> = {}): TeacherAccessResult {
  return {
    teacher: { ...ada, userId: "u1" }, userId: "u1", email: ada.email, status: "invited",
    invitation: { url: "https://colegio.mx/invitacion/abc", expiresAt: "2026-10-02T18:00:00Z", sent: true },
    ...over,
  };
}

function rowOf(name: string): HTMLElement {
  const row = screen.getByText(name).closest("li");
  if (row === null) throw new Error(`no row for ${name}`);
  return row;
}

describe("TeachersPanel — acceso al sistema", () => {
  it("each row says whether the teacher can sign in", () => {
    render(<TeachersPanel teachers={[ada, alan]} onCreate={vi.fn()} />);

    expect(within(rowOf("Ada Lovelace")).getByText("Sin acceso")).toBeInTheDocument();
    expect(within(rowOf("Alan Turing")).getByText("Con acceso")).toBeInTheDocument();
  });

  it("without the permission there is no button at all", () => {
    render(<TeachersPanel teachers={[ada, alan]} onCreate={vi.fn()} />);

    expect(screen.queryByRole("button", { name: /Dar acceso al sistema/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Reenviar invitación/ })).toBeNull();
  });

  it("gives access in one press: calls the server, flips the row and confirms where the invitation went", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockResolvedValue(outcome());
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

    expect(onGrantAccess).toHaveBeenCalledWith("t1");
    expect(await screen.findByText("Enviamos la invitación a ada@escuela.mx")).toBeInTheDocument();
    expect(within(rowOf("Ada Lovelace")).getByText("Con acceso")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reenviar invitación a Ada Lovelace" })).toBeInTheDocument();
  });

  it("with no mail provider (local), shows the link with Copiar liga", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockResolvedValue(
      outcome({ invitation: { url: "http://localhost:3002/activar?t=abc", expiresAt: "2026-10-02T18:00:00Z", sent: false } }),
    );
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

    expect(await screen.findByLabelText("Liga de acceso")).toHaveValue("http://localhost:3002/activar?t=abc");
    expect(screen.getByRole("button", { name: "Copiar liga" })).toBeInTheDocument();
    // Nothing was mailed, so nothing may say it was.
    expect(screen.queryByText(/Enviamos la invitación/)).toBeNull();
  });

  it("a sent invitation shows no link: it is in the teacher's inbox", async () => {
    const user = userEvent.setup();
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={vi.fn().mockResolvedValue(outcome())} />);

    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

    await screen.findByText("Enviamos la invitación a ada@escuela.mx");
    expect(screen.queryByLabelText("Liga de acceso")).toBeNull();
  });

  it("a suspended account is named as such and offers no button", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockResolvedValue(outcome({ teacher: alan, userId: "u2", email: alan.email, status: "suspended", invitation: null }));
    render(<TeachersPanel teachers={[alan]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Reenviar invitación a Alan Turing" }));

    expect(await within(rowOf("Alan Turing")).findByText("Acceso suspendido")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Reenviar invitación/ })).toBeNull();
    expect(screen.getByText(/La cuenta de Alan Turing está suspendida/)).toBeInTheDocument();
  });

  it("a 409 says the account belongs to another teacher", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockRejectedValue(new ConfigError(409, "already linked"));
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/ligada a otro docente/);
  });

  it("a linked teacher is offered Reenviar invitación, and nothing once the account is active", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockResolvedValue(outcome({ teacher: alan, userId: "u2", email: alan.email, status: "active", invitation: null }));
    render(<TeachersPanel teachers={[alan]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Reenviar invitación a Alan Turing" }));

    expect(onGrantAccess).toHaveBeenCalledWith("t2");
    expect(await screen.findByText(/Alan Turing ya entra al sistema/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Reenviar invitación/ })).toBeNull();
    expect(within(rowOf("Alan Turing")).getByText("Con acceso")).toBeInTheDocument();
  });

  it("a 400 shows the server's reason on the row", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockRejectedValue(new ConfigError(400, "El docente no tiene correo."));
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("El docente no tiene correo.");
    expect(within(rowOf("Ada Lovelace")).getByText("Sin acceso")).toBeInTheDocument();
  });

  it("a 403 says it is a permission matter", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockRejectedValue(new ConfigError(403, "forbidden"));
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("No tienes permiso para dar acceso a docentes.");
  });

  it("a second press while the first is in flight sends nothing more", async () => {
    const user = userEvent.setup();
    let resolve: (r: TeacherAccessResult) => void = () => {};
    const onGrantAccess = vi.fn().mockReturnValue(new Promise<TeacherAccessResult>((r) => { resolve = r; }));
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    const button = screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" });
    await user.click(button);
    await user.click(button);
    resolve(outcome());

    expect(await screen.findByText("Enviamos la invitación a ada@escuela.mx")).toBeInTheDocument();
    expect(onGrantAccess).toHaveBeenCalledTimes(1);
  });
});

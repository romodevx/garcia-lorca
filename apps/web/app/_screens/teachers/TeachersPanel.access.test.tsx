import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ConfigError } from "../../config/api";
import type { UserStatus } from "../../admin/api";
import { ReuseConfirmationRequired, type Teacher, type TeacherAccessResult } from "../../teachers/api";
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

const invitedAlan: ReadonlyMap<string, UserStatus> = new Map([["u2", "invited"]]);

function rowOf(name: string): HTMLElement {
  const row = screen.getByText(name).closest("li");
  if (row === null) throw new Error(`no row for ${name}`);
  return row;
}

describe("TeachersPanel — acceso al sistema", () => {
  it("each row says whether the teacher can sign in; a login of unknown state is just Vinculado", () => {
    render(<TeachersPanel teachers={[ada, alan]} onCreate={vi.fn()} />);

    expect(within(rowOf("Ada Lovelace")).getByText("Sin acceso")).toBeInTheDocument();
    expect(within(rowOf("Alan Turing")).getByText("Vinculado")).toBeInTheDocument();
  });

  it("a login whose state cannot be read (users:write, no users:read) is offered Reenviar invitación", () => {
    render(<TeachersPanel teachers={[alan]} onCreate={vi.fn()} onGrantAccess={vi.fn()} />);

    expect(within(rowOf("Alan Turing")).getByText("Vinculado")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reenviar invitación a Alan Turing" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Dar acceso al sistema/ })).toBeNull();
  });

  it("resending to an unreadable login that turns out active says Con acceso and shows no link", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockResolvedValue(outcome({ teacher: alan, userId: "u2", email: alan.email, status: "active", invitation: null }));
    render(<TeachersPanel teachers={[alan]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Reenviar invitación a Alan Turing" }));

    expect(onGrantAccess).toHaveBeenCalledWith("t2");
    expect(await within(rowOf("Alan Turing")).findByText("Con acceso")).toBeInTheDocument();
    expect(screen.queryByLabelText("Liga de acceso")).toBeNull();
    expect(screen.queryByRole("button", { name: /Reenviar invitación/ })).toBeNull();
  });

  it("resending to an unreadable login that is still invited flips it to Invitación enviada", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockResolvedValue(outcome({ teacher: alan, userId: "u2", email: alan.email }));
    render(<TeachersPanel teachers={[alan]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Reenviar invitación a Alan Turing" }));

    expect(await within(rowOf("Alan Turing")).findByText("Invitación enviada")).toBeInTheDocument();
  });

  it.each<[UserStatus, string, boolean]>([
    ["active", "Con acceso", false],
    // The listing says the login waits for its password, not that a mail left.
    ["invited", "Invitación pendiente", true],
    ["suspended", "Acceso suspendido", false],
    ["unknown", "Vinculado", false],
  ])("with the account's status known (%s) the row says %s", (status, label, resend) => {
    const statuses = new Map<string, UserStatus>([["u2", status]]);
    render(<TeachersPanel teachers={[alan]} onCreate={vi.fn()} onGrantAccess={vi.fn()} accountStatuses={statuses} />);

    expect(within(rowOf("Alan Turing")).getByText(label)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reenviar invitación a Alan Turing" }) !== null).toBe(resend);
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
    expect(within(rowOf("Ada Lovelace")).getByText("Invitación enviada")).toBeInTheDocument();
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
    expect(screen.getByText(/No hay correo configurado/)).toBeInTheDocument();
    // Nothing was mailed, so nothing may say it was.
    expect(screen.queryByText(/Enviamos la invitación/)).toBeNull();
    expect(within(rowOf("Ada Lovelace")).getByText("Invitación pendiente")).toBeInTheDocument();
    expect(within(rowOf("Ada Lovelace")).queryByText("Invitación enviada")).toBeNull();
  });

  it("a failed send names the reason, not a missing mail configuration", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockResolvedValue(
      outcome({ invitation: { url: "https://colegio.mx/activar?t=abc", expiresAt: "2026-10-02T18:00:00Z", sent: false, sendError: "SMTP 535 auth failed" } }),
    );
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

    expect(await screen.findByText("No pudimos enviar el correo: SMTP 535 auth failed. Copia esta liga y entrégasela.")).toBeInTheDocument();
    expect(screen.queryByText(/No hay correo configurado/)).toBeNull();
    expect(screen.getByLabelText("Liga de acceso")).toHaveValue("https://colegio.mx/activar?t=abc");
    expect(within(rowOf("Ada Lovelace")).getByText("Invitación pendiente")).toBeInTheDocument();
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
    render(<TeachersPanel teachers={[alan]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} accountStatuses={invitedAlan} />);

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

  it("an invited teacher is offered Reenviar invitación, and nothing once the account is active", async () => {
    const user = userEvent.setup();
    const onGrantAccess = vi.fn().mockResolvedValue(outcome({ teacher: alan, userId: "u2", email: alan.email, status: "active", invitation: null }));
    render(<TeachersPanel teachers={[alan]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} accountStatuses={invitedAlan} />);

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

  it("presses on two rows both go out, and each row shows its own Enviando…", async () => {
    const user = userEvent.setup();
    const bea: Teacher = { id: "t3", firstName: "Bea", lastName: "Ruiz", email: "bea@escuela.mx", userId: "" };
    const pending = new Map<string, (r: TeacherAccessResult) => void>();
    const onGrantAccess = vi.fn((id: string) => new Promise<TeacherAccessResult>((r) => { pending.set(id, r); }));
    render(<TeachersPanel teachers={[ada, bea]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));
    await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Bea Ruiz" }));

    expect(onGrantAccess).toHaveBeenCalledTimes(2);
    expect(within(rowOf("Ada Lovelace")).getByText("Enviando…")).toBeInTheDocument();
    expect(within(rowOf("Bea Ruiz")).getByText("Enviando…")).toBeInTheDocument();

    pending.get("t1")?.(outcome());
    expect(await within(rowOf("Ada Lovelace")).findByRole("button", { name: "Reenviar invitación a Ada Lovelace" })).toHaveTextContent("Reenviar invitación");
    // Ada's answer does not end Bea's wait.
    expect(within(rowOf("Bea Ruiz")).getByText("Enviando…")).toBeInTheDocument();
  });

  describe("an existing account under the address", () => {
    const question = "Ya existe una cuenta con ese correo (owner). ¿Vincularla y darle acceso de maestro?";

    it("asks on the row, in the server's words, before linking it", async () => {
      const user = userEvent.setup();
      const onGrantAccess = vi.fn().mockRejectedValueOnce(new ReuseConfirmationRequired(question)).mockResolvedValueOnce(outcome({ status: "active", invitation: null }));
      render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

      await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));

      const row = rowOf("Ada Lovelace");
      expect(await within(row).findByText(question)).toBeInTheDocument();
      expect(within(row).queryByRole("alert")).toBeNull();

      await user.click(within(row).getByRole("button", { name: "Vincular y dar acceso" }));

      expect(onGrantAccess).toHaveBeenLastCalledWith("t1", { confirmReuse: true });
      expect(await screen.findByText(/Ada Lovelace ya entra al sistema/)).toBeInTheDocument();
      expect(screen.queryByText(question)).toBeNull();
      expect(within(rowOf("Ada Lovelace")).getByText("Con acceso")).toBeInTheDocument();
    });

    it("Cancelar drops the question and sends nothing", async () => {
      const user = userEvent.setup();
      const onGrantAccess = vi.fn().mockRejectedValue(new ReuseConfirmationRequired(question));
      render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onGrantAccess={onGrantAccess} />);

      await user.click(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" }));
      await user.click(await screen.findByRole("button", { name: "Cancelar" }));

      expect(screen.queryByText(question)).toBeNull();
      expect(onGrantAccess).toHaveBeenCalledTimes(1);
      expect(screen.getByRole("button", { name: "Dar acceso al sistema a Ada Lovelace" })).toBeInTheDocument();
    });
  });
});

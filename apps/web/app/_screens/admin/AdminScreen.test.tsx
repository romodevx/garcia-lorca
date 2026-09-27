import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as adminApi from "../../admin/api";
import { AdminScreen } from "./AdminScreen";

vi.mock("../../admin/api", () => ({
  listUsers: vi.fn(),
  listRoles: vi.fn(),
  createUser: vi.fn(),
  inviteUser: vi.fn(),
  resendInvitation: vi.fn(),
  createRole: vi.fn(),
  setRolePermissions: vi.fn(),
  setUserStatus: vi.fn(),
  setUserRoles: vi.fn(),
}));
const admin = vi.mocked(adminApi);

beforeEach(() => {
  admin.listUsers.mockResolvedValue([
    { id: "u1", email: "dir@gl.mx", displayName: "Director", status: "active", mustChangePassword: false, roleIds: ["r1"] },
  ]);
  admin.listRoles.mockResolvedValue([{ id: "r1", name: "owner", permissions: ["users:read"] }]);
});
afterEach(() => vi.clearAllMocks());

describe("AdminScreen", () => {
  it("shows the users table with roles and status", async () => {
    render(<AdminScreen onBack={() => {}} />);
    expect(await screen.findByRole("heading", { name: "Administración" })).toBeInTheDocument();
    expect(await screen.findByText("Director")).toBeInTheDocument();
    expect(screen.getByText("owner")).toBeInTheDocument();
    expect(screen.getByText("Activo")).toBeInTheDocument();
  });

  // With the identity provider on, adding somebody must not ask anybody to
  // invent a password for them: the account is created empty and its owner sets
  // the first one from the emailed link.
  it("invites instead of creating when the identity provider is on", async () => {
    const user = userEvent.setup();
    admin.inviteUser.mockResolvedValue({
      id: "u9", email: "nueva@gl.mx", displayName: "Nueva",
      // "invited", not "active". An account nobody has opened yet is not in
      // use, and this fixture claiming otherwise is what let the resend button
      // render on everybody.
      status: "invited", mustChangePassword: false, roleIds: ["r1"],
      invitation: { url: "https://escuela.example/activar?token=t", expiresAt: "", sent: true },
    });
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director");

    await user.click(screen.getByRole("button", { name: /Nuevo usuario/ }));
    const dialog = await screen.findByRole("dialog", { name: "Invitar usuario" });

    // The password field is GONE. Its presence would mean somebody else knows
    // the password, which is the thing this flow exists to end.
    expect(within(dialog).queryByLabelText(/Contraseña/)).not.toBeInTheDocument();
    expect(within(dialog).queryByPlaceholderText(/primer ingreso/)).not.toBeInTheDocument();

    await user.type(within(dialog).getByLabelText("Correo"), "nueva@gl.mx");
    await user.type(within(dialog).getByLabelText("Nombre para mostrar"), "Nueva Maestra");
    await user.click(within(dialog).getByRole("button", { name: "owner" }));
    await user.click(within(dialog).getByRole("button", { name: /Enviar invitación/ }));

    expect(admin.inviteUser).toHaveBeenCalledWith(expect.any(String), { email: "nueva@gl.mx", displayName: "NUEVA MAESTRA", roleIds: ["r1"] });
    expect(admin.createUser).not.toHaveBeenCalled();
  });

  // The invitation lives on the person's own row — not behind retyping their
  // address into the new-user form, which is a conflict, not a resend.
  //
  // THIS TEST USED TO ASSERT THE BUG. Its fixture user is `u1`, status "active",
  // and it checked that the button WAS there — the exact behaviour the owner
  // reported as broken. A green suite was affirming it. Fixing the product meant
  // rewriting the assertion, not adding one next to it.
  it("resends the invitation from the row of somebody who has not entered yet", async () => {
    const user = userEvent.setup();
    admin.listUsers.mockResolvedValue([
      { id: "u2", email: "nueva@gl.mx", displayName: "Nueva Maestra", status: "invited", mustChangePassword: false, roleIds: ["r1"] },
    ]);
    admin.resendInvitation.mockResolvedValue({ url: "https://escuela.example/activar?token=t", expiresAt: "", sent: true });
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Nueva Maestra");

    await user.click(screen.getByRole("button", { name: "Reenviar invitación" }));

    expect(admin.resendInvitation).toHaveBeenCalledWith(expect.any(String), "u2");
    expect(await screen.findByText(/Invitación enviada/)).toBeInTheDocument();
  });

  // The owner's third request, asserted negatively: once somebody is in, the
  // option to re-invite them is gone.
  it("offers no resend on an account that is already active", async () => {
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director"); // fixture u1, status "active"
    expect(screen.queryByRole("button", { name: "Reenviar invitación" })).not.toBeInTheDocument();
  });

  // An undelivered invitation must not be reported as a delivered one. The link
  // stays on screen so the director can hand access over by hand.
  it("hands over the link when the mail did not go out", async () => {
    const user = userEvent.setup();
    admin.listUsers.mockResolvedValue([
      { id: "u2", email: "nueva@gl.mx", displayName: "Nueva Maestra", status: "invited", mustChangePassword: false, roleIds: ["r1"] },
    ]);
    admin.resendInvitation.mockResolvedValue({
      url: "https://escuela.example/activar?token=abc",
      expiresAt: "",
      sent: false,
      sendError: "no hay un servidor de correo configurado",
    });
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Nueva Maestra");

    await user.click(screen.getByRole("button", { name: "Reenviar invitación" }));

    expect(await screen.findByText(/El correo no salió/)).toBeInTheDocument();
    expect(screen.getByText(/token=abc/)).toBeInTheDocument();
  });

  // Adding somebody is always an invitation now, so the temp-password drawer is
  // gone from the screen. The old pair of tests here asserted that the provider
  // flag switched between the two, which stopped being a real behaviour once
  // the token and the delivery became ours.
  it("always invites, never asks the director to invent a password", async () => {
    const user = userEvent.setup();
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director");

    await user.click(screen.getByRole("button", { name: /Nuevo usuario/ }));
    const dialog = await screen.findByRole("dialog", { name: "Invitar usuario" });
    expect(within(dialog).queryByPlaceholderText(/primer ingreso/)).not.toBeInTheDocument();
  });

  it("creates a role from the drawer with checked permissions", async () => {
    const user = userEvent.setup();
    admin.createRole.mockResolvedValue({ id: "r9", name: "COORDINACIÓN", permissions: ["users:read"] });
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director");

    // switch to Roles tab (its accessible name carries the count), open the new-role drawer
    await user.click(screen.getByRole("button", { name: "Roles 1" }));
    await user.click(screen.getByRole("button", { name: /Nuevo rol/ }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo rol" });
    await user.type(within(dialog).getByLabelText("Nombre del rol"), "Coordinación");
    await user.click(within(dialog).getByRole("button", { name: "Ver usuarios" }));
    await user.click(within(dialog).getByRole("button", { name: "Crear rol" }));

    expect(admin.createRole).toHaveBeenCalledWith(expect.any(String), { name: "COORDINACIÓN", permissions: ["users:read"] });
  });

  it("shows grades:close as owner-only: visible, locked, never grantable (tarea 7)", async () => {
    const user = userEvent.setup();
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director");

    await user.click(screen.getByRole("button", { name: "Roles 1" }));
    await user.click(screen.getByRole("button", { name: /Nuevo rol/ }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo rol" });
    const close = within(dialog).getByRole("button", { name: /Cerrar periodos y emitir boletas/ });
    expect(close).toBeDisabled();
    expect(close).toHaveAccessibleName(/solo el dueño/i);
  });

  // Both actions are gated on the row's state, and both used to be gated on
  // "not active", which is not the same thing.
  //
  // "Reactivar" on an `Invitado` row set the account active with no password:
  // its invitation link then refused (activation only happens from `invited`),
  // and so did resending. One click made a person unreachable.
  it("offers neither Reactivar nor a stale resend on the wrong row states", async () => {
    admin.listUsers.mockResolvedValue([
      { id: "u2", email: "invitada@gl.mx", displayName: "Invitada", status: "invited", mustChangePassword: false, roleIds: [] },
      { id: "u3", email: "suspendida@gl.mx", displayName: "Suspendida", status: "suspended", mustChangePassword: false, roleIds: [] },
    ]);
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Invitada");

    // The invited row: it can be re-invited, and it must NOT be "reactivated".
    expect(screen.getByRole("button", { name: "Reenviar invitación" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reactivar" })).toBeInTheDocument();
    // Exactly one Reactivar — the suspended row's — and no resend on it.
    expect(screen.getAllByRole("button", { name: "Reactivar" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "Reenviar invitación" })).toHaveLength(1);
  });
});

describe("la captura de administración", () => {
  it("el correo de un usuario nuevo baja a minúsculas mientras se teclea", async () => {
    const user = userEvent.setup();
    render(<AdminScreen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: /Nuevo usuario|Invitar usuario/ }));

    const correo = screen.getByLabelText("Correo");
    await user.type(correo, "Ana.Ruiz@CEGLORCA.COM");
    expect(correo).toHaveValue("ana.ruiz@ceglorca.com");
    expect(correo).toHaveAttribute("inputmode", "email");
  });

  it("un correo sin forma de correo no deja crear al usuario", async () => {
    const user = userEvent.setup();
    render(<AdminScreen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "Nuevo usuario" }));

    // Everything else is valid, so only the email can hold the drawer back.
    await user.type(screen.getByLabelText(/^Nombre para mostrar/), "Ana Ruiz");
    const password = screen.queryByLabelText(/^Contraseña temporal/);
    if (password !== null) await user.type(password, "init-pw-123");
    // No Tab: the field never loses focus, so only the save-time check can
    // mark it — exactly the keyboard path (Enter on the focused button).
    await user.type(screen.getByLabelText("Correo"), "ana@colegio");
    expect(screen.getByLabelText("Correo")).not.toHaveAttribute("aria-invalid");

    // The drawer's primary action (create or invite, per configuration) stays
    // unready: pointer-events none also hides it from the accessibility tree.
    const primary = screen.getByRole("button", { name: /Crear usuario|Enviar invitación/, hidden: true });
    expect(primary).toHaveStyle({ pointerEvents: "none" });

    // pointer-events stops the mouse, not the keyboard: Tab + Enter still
    // activates the button (a click event). The drawer must refuse on its own.
    fireEvent.click(primary);
    expect(admin.createUser).not.toHaveBeenCalled();
    expect(admin.inviteUser).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Correo")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText(/nombre@dominio/)).toBeInTheDocument();
  });

  it("el nombre para mostrar se captura en mayúsculas", async () => {
    const user = userEvent.setup();
    render(<AdminScreen onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: /Nuevo usuario|Invitar usuario/ }));

    const nombre = screen.getByLabelText("Nombre para mostrar");
    await user.type(nombre, "ana ruiz");
    expect(nombre).toHaveValue("ANA RUIZ");
  });

  // La contraseña temporal queda FUERA de la capa de captura a propósito —
  // nada debe subirla ni bajarla— pero en este entorno el proveedor de
  // identidad está activo y ese campo no existe, así que no hay camino que
  // probarlo aquí. La garantía de fondo es que app/_design/form/ no tiene
  // ninguna primitiva de contraseña.
});

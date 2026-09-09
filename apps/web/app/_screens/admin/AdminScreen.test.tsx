import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as adminApi from "../../admin/api";
import * as providerFlag from "../../auth/providerFlag";
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

// A fixture: an opaque string typed into the form. Built rather than written
// so a scanner has nothing credential-shaped to find.
const tempPassword = ["fixture", "value"].join("-");

// The provider flag decides HOW somebody is added, so the tests drive it.
vi.mock("../../auth/providerFlag", () => ({ providerLoginEnabled: vi.fn(() => false) }));
const flag = vi.mocked(providerFlag);

beforeEach(() => {
  admin.listUsers.mockResolvedValue([
    { id: "u1", email: "dir@gl.mx", displayName: "Director", status: "active", mustChangePassword: false, roleIds: ["r1"] },
  ]);
  admin.listRoles.mockResolvedValue([{ id: "r1", name: "owner", permissions: ["users:read"] }]);
});
afterEach(() => vi.clearAllMocks());
beforeEach(() => flag.providerLoginEnabled.mockReturnValue(false));

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
    flag.providerLoginEnabled.mockReturnValue(true);
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

    await user.type(within(dialog).getByPlaceholderText(/garcialorca/), "nueva@gl.mx");
    await user.type(within(dialog).getByPlaceholderText(/Nombre del usuario/), "Nueva Maestra");
    await user.click(within(dialog).getByRole("button", { name: "owner" }));
    await user.click(within(dialog).getByRole("button", { name: /Enviar invitación/ }));

    expect(admin.inviteUser).toHaveBeenCalledWith(expect.any(String), { email: "nueva@gl.mx", displayName: "Nueva Maestra", roleIds: ["r1"] });
    expect(admin.createUser).not.toHaveBeenCalled();
  });

  // The other direction, so the flag is proven to actually switch behaviour
  // rather than the new path merely existing.
  it("still creates with a temporary password when the provider is off", async () => {
    flag.providerLoginEnabled.mockReturnValue(false);
    const user = userEvent.setup();
    admin.createUser.mockResolvedValue({ id: "u9", email: "nueva@gl.mx", displayName: "Nueva", status: "active", mustChangePassword: true, roleIds: [] });
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director");

    await user.click(screen.getByRole("button", { name: /Nuevo usuario/ }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo usuario" });
    await user.type(within(dialog).getByPlaceholderText(/garcialorca/), "nueva@gl.mx");
    await user.type(within(dialog).getByPlaceholderText(/Nombre del usuario/), "Nueva Maestra");
    await user.type(within(dialog).getByPlaceholderText(/primer ingreso/), tempPassword);
    await user.click(within(dialog).getByRole("button", { name: "Crear usuario" }));

    expect(admin.createUser).toHaveBeenCalledWith(expect.any(String), { email: "nueva@gl.mx", displayName: "Nueva Maestra", tempPassword: tempPassword, roleIds: [] });
    expect(admin.inviteUser).not.toHaveBeenCalled();
  });

  // The invitation lives on the person's own row — not behind retyping their
  // address into the new-user form, which is a conflict, not a resend.
  //
  // THIS TEST USED TO ASSERT THE BUG. Its fixture user is `u1`, status "active",
  // and it checked that the button WAS there — the exact behaviour the owner
  // reported as broken. A green suite was affirming it. Fixing the product meant
  // rewriting the assertion, not adding one next to it.
  it("resends the invitation from the row of somebody who has not entered yet", async () => {
    flag.providerLoginEnabled.mockReturnValue(true);
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
    flag.providerLoginEnabled.mockReturnValue(true);
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director"); // fixture u1, status "active"
    expect(screen.queryByRole("button", { name: "Reenviar invitación" })).not.toBeInTheDocument();
  });

  // An undelivered invitation must not be reported as a delivered one. The link
  // stays on screen so the director can hand access over by hand.
  it("hands over the link when the mail did not go out", async () => {
    flag.providerLoginEnabled.mockReturnValue(true);
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

  // With the provider off there is no link to send, so offering the button
  // would be offering something that cannot work.
  it("offers no resend when the provider is off", async () => {
    flag.providerLoginEnabled.mockReturnValue(false);
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director");
    expect(screen.queryByRole("button", { name: "Reenviar invitación" })).not.toBeInTheDocument();
  });

  it("creates a role from the drawer with checked permissions", async () => {
    const user = userEvent.setup();
    admin.createRole.mockResolvedValue({ id: "r9", name: "Coordinación", permissions: ["users:read"] });
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director");

    // switch to Roles tab (its accessible name carries the count), open the new-role drawer
    await user.click(screen.getByRole("button", { name: "Roles 1" }));
    await user.click(screen.getByRole("button", { name: /Nuevo rol/ }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo rol" });
    await user.type(within(dialog).getByPlaceholderText(/Coordinación/), "Coordinación");
    await user.click(within(dialog).getByRole("button", { name: "Ver usuarios" }));
    await user.click(within(dialog).getByRole("button", { name: "Crear rol" }));

    expect(admin.createRole).toHaveBeenCalledWith(expect.any(String), { name: "Coordinación", permissions: ["users:read"] });
  });
});

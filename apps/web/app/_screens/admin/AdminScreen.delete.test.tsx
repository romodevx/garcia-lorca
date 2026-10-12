import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as adminApi from "../../admin/api";
import { canDeleteUsers } from "../../admin/permissions";
import { AdminScreen } from "./AdminScreen";

// T2 — deleting a user from the list. Irreversible from the UI, so it asks
// first, in a real dialog (APG): focus goes in, Esc and Cancelar leave without
// touching anything, and focus comes back to the row's button.

vi.mock("../../admin/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../admin/api")>();
  return {
    AdminError: real.AdminError,
    listUsers: vi.fn(),
    listRoles: vi.fn(),
    createUser: vi.fn(),
    inviteUser: vi.fn(),
    resendInvitation: vi.fn(),
    createRole: vi.fn(),
    setRolePermissions: vi.fn(),
    setUserStatus: vi.fn(),
    setUserRoles: vi.fn(),
    deleteUser: vi.fn(),
  };
});
const admin = vi.mocked(adminApi);

beforeEach(() => {
  admin.listUsers.mockResolvedValue([
    { id: "u1", email: "dir@gl.mx", displayName: "Director", status: "active", mustChangePassword: false, roleIds: ["r1"] },
    { id: "u2", email: "maestra@gl.mx", displayName: "Maestra Rojas", status: "active", mustChangePassword: false, roleIds: [] },
  ]);
  admin.listRoles.mockResolvedValue([{ id: "r1", name: "owner", permissions: ["users:read"] }]);
});
afterEach(() => vi.clearAllMocks());

function renderAsOwner(): void {
  render(<AdminScreen onBack={() => {}} canDeleteUsers currentUserId="u1" />);
}

describe("canDeleteUsers", () => {
  it("is users:delete and nothing else", () => {
    expect(canDeleteUsers(["users:delete"])).toBe(true);
    expect(canDeleteUsers(["users:write", "users:read"])).toBe(false);
    expect(canDeleteUsers(undefined)).toBe(false);
  });
});

describe("AdminScreen · eliminar usuario", () => {
  it("offers no delete to a caller without users:delete", async () => {
    render(<AdminScreen onBack={() => {}} currentUserId="u1" />);
    await screen.findByText("Maestra Rojas");

    expect(screen.queryByRole("button", { name: /Eliminar a/ })).not.toBeInTheDocument();
  });

  it("offers delete on other people's rows, never on your own", async () => {
    renderAsOwner();
    await screen.findByText("Maestra Rojas");

    expect(screen.getByRole("button", { name: "Eliminar a Maestra Rojas" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Eliminar a Director" })).not.toBeInTheDocument();
  });

  it("asks first, and Cancelar leaves everything as it was", async () => {
    const user = userEvent.setup();
    renderAsOwner();
    await screen.findByText("Maestra Rojas");
    const trigger = screen.getByRole("button", { name: "Eliminar a Maestra Rojas" });

    await user.click(trigger);
    const dialog = await screen.findByRole("dialog", { name: "Eliminar usuario" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(within(dialog).getByText(/Maestra Rojas/)).toBeInTheDocument();
    expect(within(dialog).getByText(/historial/)).toBeInTheDocument();
    expect(dialog.contains(document.activeElement)).toBe(true);

    await user.click(within(dialog).getByRole("button", { name: "Cancelar" }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(admin.deleteUser).not.toHaveBeenCalled();
    expect(trigger).toHaveFocus();
  });

  it("Esc closes the dialog without deleting", async () => {
    const user = userEvent.setup();
    renderAsOwner();
    await screen.findByText("Maestra Rojas");

    await user.click(screen.getByRole("button", { name: "Eliminar a Maestra Rojas" }));
    await screen.findByRole("dialog", { name: "Eliminar usuario" });
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(admin.deleteUser).not.toHaveBeenCalled();
  });

  it("deletes on confirmation, reloads the list and says so", async () => {
    const user = userEvent.setup();
    admin.deleteUser.mockResolvedValue(undefined);
    renderAsOwner();
    await screen.findByText("Maestra Rojas");

    await user.click(screen.getByRole("button", { name: "Eliminar a Maestra Rojas" }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar usuario" });
    admin.listUsers.mockResolvedValue([
      { id: "u1", email: "dir@gl.mx", displayName: "Director", status: "active", mustChangePassword: false, roleIds: ["r1"] },
    ]);
    await user.click(within(dialog).getByRole("button", { name: "Eliminar usuario" }));

    expect(admin.deleteUser).toHaveBeenCalledWith(expect.any(String), "u2");
    expect(await screen.findByText(/Usuario eliminado · Maestra Rojas/)).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Maestra Rojas")).not.toBeInTheDocument());
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("keeps the dialog open and shows the server's reason when refused", async () => {
    const user = userEvent.setup();
    admin.deleteUser.mockRejectedValue(new adminApi.AdminError(409, "no se puede eliminar al último director activo"));
    renderAsOwner();
    await screen.findByText("Maestra Rojas");

    await user.click(screen.getByRole("button", { name: "Eliminar a Maestra Rojas" }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar usuario" });
    await user.click(within(dialog).getByRole("button", { name: "Eliminar usuario" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("No se puede eliminar al último director activo.");
    expect(screen.getByRole("dialog", { name: "Eliminar usuario" })).toBeInTheDocument();
  });

  // The row is gone (nobody can sign in) but the identity provider did not
  // answer: say exactly that and keep the button for the retry, which only
  // repeats the provider half.
  it("says the account is closed but the provider is pending, and retries", async () => {
    const user = userEvent.setup();
    admin.deleteUser.mockRejectedValueOnce(new adminApi.AdminError(500, "internal error", "DELETION_PENDING"));
    admin.deleteUser.mockResolvedValueOnce(undefined);
    renderAsOwner();
    await screen.findByText("Maestra Rojas");

    await user.click(screen.getByRole("button", { name: "Eliminar a Maestra Rojas" }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar usuario" });
    await user.click(within(dialog).getByRole("button", { name: "Eliminar usuario" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/ya no puede entrar.*proveedor/);

    await user.click(within(dialog).getByRole("button", { name: "Eliminar usuario" }));
    expect(admin.deleteUser).toHaveBeenCalledTimes(2);
    expect(await screen.findByText(/Usuario eliminado · Maestra Rojas/)).toBeInTheDocument();
  });

  // Bug 2026-10-12 (dev): the provider half failed EVERY time (missing grant),
  // so the retry never succeeded, the dialog never closed, and the deleted row
  // stayed in the list until a page reload. The row is gone the moment the
  // account is blocked, so the list refreshes then.
  it("refreshes the list as soon as the account is blocked, even if the provider is pending", async () => {
    const user = userEvent.setup();
    admin.deleteUser.mockRejectedValue(new adminApi.AdminError(500, "internal error", "DELETION_PENDING"));
    renderAsOwner();
    await screen.findByText("Maestra Rojas");
    admin.listUsers.mockResolvedValue([
      { id: "u1", email: "dir@gl.mx", displayName: "Director", status: "active", mustChangePassword: false, roleIds: ["r1"] },
    ]);

    await user.click(screen.getByRole("button", { name: "Eliminar a Maestra Rojas" }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar usuario" });
    await user.click(within(dialog).getByRole("button", { name: "Eliminar usuario" }));
    await within(dialog).findByRole("alert");

    await waitFor(() => expect(screen.queryByRole("button", { name: "Eliminar a Maestra Rojas" })).not.toBeInTheDocument());
  });

  // A second pending in a row means retrying will not fix it: say so, and let
  // the director close instead of pressing a button that can never work.
  it("after a second pending, stops offering the retry and lets the dialog close", async () => {
    const user = userEvent.setup();
    admin.deleteUser.mockRejectedValue(new adminApi.AdminError(500, "internal error", "DELETION_PENDING"));
    renderAsOwner();
    await screen.findByText("Maestra Rojas");

    await user.click(screen.getByRole("button", { name: "Eliminar a Maestra Rojas" }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar usuario" });
    await user.click(within(dialog).getByRole("button", { name: "Eliminar usuario" }));
    await within(dialog).findByRole("alert");
    await user.click(within(dialog).getByRole("button", { name: "Eliminar usuario" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/reintentar no lo resuelve/i);
    expect(within(dialog).queryByRole("button", { name: "Eliminar usuario" })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Entendido" }));
    expect(admin.deleteUser).toHaveBeenCalledTimes(2);
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Eliminar usuario" })).not.toBeInTheDocument());
    expect(await screen.findByText(/Usuario eliminado · Maestra Rojas/)).toBeInTheDocument();
  });
});

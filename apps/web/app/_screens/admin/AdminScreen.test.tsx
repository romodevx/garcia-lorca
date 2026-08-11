import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as adminApi from "../../admin/api";
import { AdminScreen } from "./AdminScreen";

vi.mock("../../admin/api", () => ({
  listUsers: vi.fn(),
  listRoles: vi.fn(),
  createUser: vi.fn(),
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

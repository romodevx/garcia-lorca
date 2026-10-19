import { render, screen, within } from "@testing-library/react";
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

/**
 * ISSUE-CC-114: the permission chips were bare buttons; colour alone said
 * which were chosen, so a screen reader heard every one the same.
 */
describe("AdminScreen — the permission picker says what is chosen", () => {
  it("exposes each permission as a toggle with aria-pressed", async () => {
    const user = userEvent.setup();
    render(<AdminScreen onBack={() => {}} />);
    await screen.findByText("Director");
    await user.click(screen.getByRole("button", { name: "Roles 1" }));
    await user.click(screen.getByRole("button", { name: /Nuevo rol/ }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo rol" });

    const permission = within(dialog).getByRole("button", { name: "Ver usuarios" });
    expect(permission).toHaveAttribute("aria-pressed", "false");
    await user.click(permission);
    expect(permission).toHaveAttribute("aria-pressed", "true");
  });
});

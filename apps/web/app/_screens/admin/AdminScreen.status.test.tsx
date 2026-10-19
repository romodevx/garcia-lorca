import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as adminApi from "../../admin/api";
import { AdminScreen } from "./AdminScreen";

// ISSUE-CC-117 / CC-114 — Suspender is never offered on your own row (iam
// refuses it anyway: this is the courtesy, that is the rule), and like
// Eliminar it names the person it acts on, so a screen reader hears whom.
// ISSUE-CC-115 — the guardian role is not offered when inviting by hand: it is
// given only from "Dar acceso a la app" on the pupil's record.

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

const GUARDIAN_ROLE_ID = "00000000-0000-0000-0000-0000000000a2";

beforeEach(() => {
  admin.listUsers.mockResolvedValue([
    { id: "u1", email: "dir@gl.mx", displayName: "Director", status: "active", mustChangePassword: false, roleIds: ["r1"] },
    { id: "u2", email: "maestra@gl.mx", displayName: "Maestra Rojas", status: "active", mustChangePassword: false, roleIds: [] },
    { id: "u3", email: "susp@gl.mx", displayName: "Suspendida", status: "suspended", mustChangePassword: false, roleIds: [] },
  ]);
  admin.listRoles.mockResolvedValue([
    { id: "r1", name: "owner", permissions: ["users:read"] },
    { id: "r2", name: "maestro", permissions: ["teaching:read"] },
    { id: GUARDIAN_ROLE_ID, name: "guardian", permissions: ["family:read"] },
  ]);
});
afterEach(() => vi.clearAllMocks());

describe("AdminScreen · suspender", () => {
  it("offers Suspender on other people's rows, never on your own", async () => {
    render(<AdminScreen onBack={() => {}} currentUserId="u1" />);
    await screen.findByText("Maestra Rojas");

    expect(screen.getByRole("button", { name: "Suspender a Maestra Rojas" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Suspender a Director" })).not.toBeInTheDocument();
  });

  it("names the person on Reactivar too", async () => {
    render(<AdminScreen onBack={() => {}} currentUserId="u1" />);
    await screen.findByText("Suspendida");

    expect(screen.getByRole("button", { name: "Reactivar a Suspendida" })).toBeInTheDocument();
  });

  it("still suspends somebody else", async () => {
    const user = userEvent.setup();
    admin.setUserStatus.mockResolvedValue({ id: "u2", email: "maestra@gl.mx", displayName: "Maestra Rojas", status: "suspended", mustChangePassword: false, roleIds: [] });
    render(<AdminScreen onBack={() => {}} currentUserId="u1" />);

    await user.click(await screen.findByRole("button", { name: "Suspender a Maestra Rojas" }));

    expect(admin.setUserStatus).toHaveBeenCalledWith(expect.any(String), "u2", "suspended");
  });
});

describe("AdminScreen · rol guardian", () => {
  it("does not offer the guardian role when inviting by hand", async () => {
    const user = userEvent.setup();
    render(<AdminScreen onBack={() => {}} currentUserId="u1" />);
    await screen.findByText("Director");

    await user.click(screen.getByRole("button", { name: /Nuevo usuario/ }));
    const dialog = await screen.findByRole("dialog", { name: "Invitar usuario" });

    expect(within(dialog).getByRole("button", { name: "maestro" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "guardian" })).not.toBeInTheDocument();
  });
});

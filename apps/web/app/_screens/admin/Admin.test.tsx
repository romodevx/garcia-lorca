import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as adminApi from "../../admin/api";
import * as teachersApi from "../../teachers/api";
import { Admin } from "./Admin";

// The screen talks to the gateway only through the admin + config api clients,
// so we mock them wholesale and assert the calls (the fetch itself is covered by
// app/admin/api.test.ts). Neutral literals only.
vi.mock("../../admin/api", () => ({
  listUsers: vi.fn(),
  createUser: vi.fn(),
  setUserStatus: vi.fn(),
  setUserRoles: vi.fn(),
  listRoles: vi.fn(),
  createRole: vi.fn(),
  setRolePermissions: vi.fn(),
  linkTeacherUser: vi.fn(),
}));

vi.mock("../../teachers/api", () => ({
  listTeachers: vi.fn(),
}));

const admin = vi.mocked(adminApi);
const teachers = vi.mocked(teachersApi);

beforeEach(() => {
  admin.listUsers.mockResolvedValue([]);
  admin.listRoles.mockResolvedValue([]);
  teachers.listTeachers.mockResolvedValue([]);
});

afterEach(() => vi.clearAllMocks());

describe("Admin — roles", () => {
  it("creates a role with the checked permissions", async () => {
    const user = userEvent.setup();
    admin.createRole.mockResolvedValue({ id: "r1", name: "Coordinación", permissions: ["users:read", "roles:read"] });

    render(<Admin onBack={() => {}} />);

    const nameInput = await screen.findByLabelText("Nombre del rol");
    await user.type(nameInput, "Coordinación");

    // Check two permissions from the catalog checklist.
    await user.click(screen.getByLabelText("Ver usuarios"));
    await user.click(screen.getByLabelText("Ver roles"));

    await user.click(screen.getByRole("button", { name: "Crear rol" }));

    await waitFor(() =>
      expect(admin.createRole).toHaveBeenCalledWith(expect.any(String), {
        name: "Coordinación",
        permissions: ["users:read", "roles:read"],
      }),
    );
  });

  it("renders a role whose permissions come back null without crashing the panel", async () => {
    // Regression: a role created with no permissions used to serialize as null;
    // the panel did role.permissions.length and took the whole Admin surface
    // down ("This page couldn't load"). It must degrade to 0 instead.
    admin.listRoles.mockResolvedValue([{ id: "r9", name: "Coordinador", permissions: null as unknown as string[] }]);

    render(<Admin onBack={() => {}} />);

    // The panel renders (before the fix it threw on role.permissions.length and
    // the whole surface went to "This page couldn't load"). The count degrades to 0.
    expect(await screen.findByText(/0 permiso\(s\)/)).toBeInTheDocument();
    expect(screen.getAllByText("Coordinador").length).toBeGreaterThan(0);
  });
});

describe("Admin — users", () => {
  it("creates a user with email, display name, temp password and roles", async () => {
    const user = userEvent.setup();
    admin.listRoles.mockResolvedValue([{ id: "r1", name: "Coordinación", permissions: [] }]);
    admin.createUser.mockResolvedValue({
      id: "u1",
      email: "director@example.com",
      displayName: "Directora",
      status: "active",
      mustChangePassword: true,
      roleIds: ["r1"],
    });

    render(<Admin onBack={() => {}} />);

    await user.type(await screen.findByLabelText("Correo"), "director@example.com");
    await user.type(screen.getByLabelText("Nombre para mostrar"), "Directora");
    await user.type(screen.getByLabelText("Contraseña temporal"), "init-pw-123");
    // Select the role via its multi-select checkbox (label = role name).
    await user.click(screen.getByLabelText("Coordinación"));

    await user.click(screen.getByRole("button", { name: "Crear usuario" }));

    await waitFor(() =>
      expect(admin.createUser).toHaveBeenCalledWith(expect.any(String), {
        email: "director@example.com",
        displayName: "Directora",
        tempPassword: "init-pw-123",
        roleIds: ["r1"],
      }),
    );
  });

  it("baja a minúsculas el correo del usuario nuevo mientras se teclea", async () => {
    const user = userEvent.setup();
    admin.listUsers.mockResolvedValue([]);
    admin.listRoles.mockResolvedValue([]);

    render(<Admin onBack={() => {}} />);

    const correo = await screen.findByLabelText("Correo");
    await user.type(correo, "Maestra.Ruiz@Colegio.MX");

    expect(correo).toHaveValue("maestra.ruiz@colegio.mx");
  });

  it("renders a user whose roleIds come back null without crashing the panel", async () => {
    // Regression: a user created with no roles used to serialize roleIds as null;
    // the panel did user.roleIds.length / new Set(user.roleIds) and took the whole
    // Admin surface down ("This page couldn't load"). It must show "Sin roles".
    admin.listUsers.mockResolvedValue([
      { id: "u9", email: "noroles@example.com", displayName: "Sin Roles", status: "active", mustChangePassword: true, roleIds: null as unknown as string[] },
    ]);

    render(<Admin onBack={() => {}} />);

    expect(await screen.findByText("Sin Roles")).toBeInTheDocument();
    expect(screen.getByText("Sin roles")).toBeInTheDocument();
  });

  it("suspends an active user", async () => {
    const user = userEvent.setup();
    admin.listUsers.mockResolvedValue([
      { id: "u1", email: "director@example.com", displayName: "Directora", status: "active", mustChangePassword: false, roleIds: [] },
    ]);
    admin.setUserStatus.mockResolvedValue({
      id: "u1",
      email: "director@example.com",
      displayName: "Directora",
      status: "suspended",
      mustChangePassword: false,
      roleIds: [],
    });

    render(<Admin onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: "Suspender a Directora" }));

    await waitFor(() => expect(admin.setUserStatus).toHaveBeenCalledWith(expect.any(String), "u1", "suspended"));
  });
});

describe("Admin — teacher link", () => {
  it("links a teacher to a user", async () => {
    const user = userEvent.setup();
    teachers.listTeachers.mockResolvedValue([
      { id: "t1", firstName: "Ada", lastName: "Lovelace", email: "ada@example.com", userId: "" },
    ]);
    admin.listUsers.mockResolvedValue([
      { id: "u1", email: "director@example.com", displayName: "Directora", status: "active", mustChangePassword: false, roleIds: [] },
    ]);
    admin.linkTeacherUser.mockResolvedValue({ id: "t1", firstName: "Ada", lastName: "Lovelace", email: "ada@example.com", userId: "u1" });

    render(<Admin onBack={() => {}} />);

    await user.selectOptions(await screen.findByLabelText("Profesor"), "t1");
    await user.selectOptions(screen.getByLabelText("Usuario"), "u1");
    await user.click(screen.getByRole("button", { name: "Vincular" }));

    await waitFor(() => expect(admin.linkTeacherUser).toHaveBeenCalledWith(expect.any(String), "t1", "u1"));
  });
});

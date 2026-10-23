import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { me as apiMe } from "./auth/api";

/*
 * ADR 0041: the "Familias" audience of /avisos is offered only with
 * module_families AND module_students, and to a caller holding students:read
 * (the families' list needs it). Read here, never by the screen.
 */

vi.mock("next/navigation", () => ({
  useRouter: () => fakeRouter,
  usePathname: () => useFakePathname(),
}));

vi.mock("./auth/api", () => ({
  login: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn(),
  refresh: vi.fn(() => Promise.reject(new Error("no session"))),
  me: vi.fn(),
}));

vi.mock("./comms/api", () => ({
  announce: vi.fn(),
  getBirthdayTemplate: vi.fn().mockResolvedValue({ title: "Feliz", body: "Hola", isDefault: true, updatedAt: "" }),
  updateBirthdayTemplate: vi.fn(),
  listNotifications: vi.fn().mockResolvedValue([]),
}));

vi.mock("./config/api", async (importOriginal) => ({
  ConfigError: (await importOriginal<typeof import("./config/api")>()).ConfigError,
  listCycles: vi.fn().mockResolvedValue([]),
  listLevels: vi.fn().mockResolvedValue([]),
  listLabelledGroups: vi.fn().mockResolvedValue([]),
}));

vi.mock("./students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]), listStudentsWithRooms: vi.fn().mockResolvedValue([]) }));

vi.mock("./families/api", () => ({ listFamilies: vi.fn().mockResolvedValue([]) }));

const ON = { module_comms: true, module_announcements_view: true, module_students: true, module_families: true } as const;

function signIn(permissions: string[]): void {
  vi.mocked(apiMe).mockResolvedValue({ userId: "u1", email: "owner@example.com", permissions });
}

beforeEach(() => {
  resetFakeRouter("/avisos");
  signIn(["comms:read", "comms:write", "students:read"]);
});
afterEach(() => vi.clearAllMocks());

describe("/avisos and the module_families flag", () => {
  it("ON with students:read: Familias is an audience", async () => {
    render(<Providers flags={ON}><AppShell /></Providers>);
    expect(await screen.findByRole("button", { name: "Familias" })).toBeInTheDocument();
  });

  it.each([
    ["module_families off", { ...ON, module_families: false }, ["comms:read", "comms:write", "students:read"]],
    ["module_students off", { ...ON, module_students: false }, ["comms:read", "comms:write", "students:read"]],
    ["no students:read", ON, ["comms:read", "comms:write"]],
  ])("%s: the composer is today's", async (_label, flags, permissions) => {
    signIn(permissions);
    render(<Providers flags={flags}><AppShell /></Providers>);
    expect(await screen.findByRole("button", { name: "Grupos" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Familias" })).toBeNull();
  });
});

import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { me as apiMe } from "./auth/api";
import { listFamilies, listFamilySuggestions } from "./families/api";

/*
 * module_families (ADR 0041) is read once here, with module_students, and
 * handed to Alumnos and the pupil's file; no screen reads the flag itself.
 * OFF, nothing of families renders and nothing is asked of the gateway.
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

vi.mock("./config/api", async (importOriginal) => ({
  ConfigError: (await importOriginal<typeof import("./config/api")>()).ConfigError,
  listCycles: vi.fn().mockResolvedValue([]),
  listLevels: vi.fn().mockResolvedValue([]),
  listGradeLevels: vi.fn().mockResolvedValue([]),
  listLabelledGroups: vi.fn().mockResolvedValue([]),
}));

vi.mock("./students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./students/api")>()),
  listStudents: vi.fn().mockResolvedValue([
    { id: "s1", curp: "CURP1", firstName: "Ana", lastName: "Arce", level: "Primaria", levelId: "", gradeGroup: "A", guardianName: "", admissionStatus: "enrolled" },
  ]),
  listStudentsWithRooms: vi.fn().mockResolvedValue([]),
  listEnrollments: vi.fn().mockResolvedValue([]),
}));

vi.mock("./families/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./families/api")>()),
  listFamilies: vi.fn().mockResolvedValue([]),
  listFamilySuggestions: vi.fn().mockResolvedValue([]),
}));

beforeEach(() => {
  resetFakeRouter("/alumnos");
  vi.mocked(apiMe).mockResolvedValue({ userId: "u1", email: "owner@example.com", permissions: ["students:read", "guardians:read", "guardians:write"] });
});
afterEach(() => vi.clearAllMocks());

describe("Estudiantes and the module_families flag", () => {
  it("OFF: no Familias view and no request for families", async () => {
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByText("Arce Ana")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Familias" })).toBeNull();
    expect(listFamilies).not.toHaveBeenCalled();
    expect(listFamilySuggestions).not.toHaveBeenCalled();
  });

  it("ON without module_students: still nothing (it requires it)", async () => {
    render(
      <Providers flags={{ module_families: true }}>
        <AppShell />
      </Providers>,
    );
    await screen.findAllByText(/./);
    expect(screen.queryByRole("button", { name: "Familias" })).toBeNull();
    expect(listFamilies).not.toHaveBeenCalled();
  });

  it("ON: Alumnos offers the Familias view", async () => {
    render(
      <Providers flags={{ module_students: true, module_families: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByRole("button", { name: "Familias" })).toBeInTheDocument();
    expect(listFamilies).toHaveBeenCalled();
  });
});

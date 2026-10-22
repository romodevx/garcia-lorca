import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { me as apiMe } from "./auth/api";

/*
 * module_import_review (PLAN-import-avisos) is read once here and handed to the
 * import, the list and the file; no screen reads the flag itself.
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
    { id: "s1", curp: "CURP1", firstName: "Ana", lastName: "Arce", level: "Primaria", levelId: "", gradeGroup: "A", guardianName: "", admissionStatus: "enrolled", openIssues: 2, draft: false },
  ]),
  listStudentsWithRooms: vi.fn().mockResolvedValue([]),
  listEnrollments: vi.fn().mockResolvedValue([]),
}));

beforeEach(() => {
  resetFakeRouter("/alumnos");
  vi.mocked(apiMe).mockResolvedValue({ userId: "u1", email: "owner@example.com", permissions: ["students:read"] });
});
afterEach(() => vi.clearAllMocks());

describe("Estudiantes and the module_import_review flag", () => {
  it("OFF: no import warnings on the list", async () => {
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByText("Arce Ana")).toBeInTheDocument();
    expect(screen.queryByLabelText("2 avisos de importación")).toBeNull();
  });

  it("ON: the list shows them", async () => {
    render(
      <Providers flags={{ module_students: true, module_import_review: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByLabelText("2 avisos de importación")).toBeInTheDocument();
  });
});

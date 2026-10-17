import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { me as apiMe } from "./auth/api";
import { listGradeLevels } from "./config/api";

/*
 * module_config_setup picks the view of /configuracion (PLAN-redesign-configuracion
 * §5, §6): ON paints the "Puesta en marcha" steps, OFF paints today's catalog.
 * The choice lives here, beside the other flag reads, never inside Catalog.
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
  listGroups: vi.fn().mockResolvedValue([]),
  listLabelledGroups: vi.fn().mockResolvedValue([]),
  listLevels: vi.fn().mockResolvedValue([]),
  listGradeLevels: vi.fn().mockResolvedValue([]),
  listTerms: vi.fn().mockResolvedValue([]),
  listSubjects: vi.fn().mockResolvedValue([]),
  getSchoolProfile: vi.fn().mockResolvedValue({ profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" }, found: false }),
  listLevelTrash: vi.fn().mockResolvedValue([]),
}));

vi.mock("./finance/api", () => ({
  listTuitionPlans: vi.fn().mockResolvedValue([]),
}));

beforeEach(() => {
  resetFakeRouter("/configuracion");
  vi.mocked(apiMe).mockResolvedValue({ userId: "u1", email: "owner@example.com", permissions: ["config:read", "config:write"] });
});
afterEach(() => vi.clearAllMocks());

describe("/configuracion and the module_config_setup flag", () => {
  it("with the flag ON paints the setup steps", async () => {
    render(
      <Providers flags={{ module_config: true, module_config_setup: true }}>
        <AppShell />
      </Providers>,
    );

    expect(await screen.findByRole("navigation", { name: "Puesta en marcha" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Configuración académica" })).toBeInTheDocument();
    expect(screen.queryByText("Ciclos, niveles, grados, materias, grupos y periodos")).toBeNull();
  });

  it("with the flag OFF paints today's catalog, which never reads every grade", async () => {
    render(
      <Providers flags={{ module_config: true }}>
        <AppShell />
      </Providers>,
    );

    expect(await screen.findByText("Ciclos, niveles, grados, materias, grupos y periodos")).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Puesta en marcha" })).toBeNull();
    expect(vi.mocked(listGradeLevels)).not.toHaveBeenCalledWith(expect.any(String));
  });
});

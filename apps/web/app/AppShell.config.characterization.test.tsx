import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers, fakePath, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { me as apiMe } from "./auth/api";
import { listCycles } from "./config/api";

/*
 * Characterization test (2026-10-09), frozen before the Configuración académica
 * redesign: behavior.md §0 S0.1. With module_config OFF, /configuracion never
 * mounts the catalog; it paints the "settings" module placeholder instead.
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
  getSchoolProfile: vi.fn().mockResolvedValue({ profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" }, found: false }),
  listLevelTrash: vi.fn().mockResolvedValue([]),
}));

vi.mock("./finance/api", () => ({
  listTuitionPlans: vi.fn().mockResolvedValue([]),
}));

const READER_ME = { userId: "u1", email: "owner@example.com", permissions: ["config:read"] };

beforeEach(() => {
  resetFakeRouter("/configuracion");
  vi.mocked(apiMe).mockResolvedValue(READER_ME);
});
afterEach(() => vi.clearAllMocks());

describe("/configuracion and the module_config flag (S0.1)", () => {
  it("with the flag OFF paints the Configuración placeholder, never the catalog, and its way back goes home", async () => {
    const user = userEvent.setup();
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>,
    );

    expect(await screen.findByRole("heading", { name: "Módulo «Configuración»" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Configuración académica" })).toBeNull();
    expect(screen.queryByText("Ciclo escolar")).toBeNull();
    expect(vi.mocked(listCycles)).not.toHaveBeenCalled();
    expect(fakePath()).toBe("/configuracion");

    await user.click(screen.getByRole("button", { name: "Volver al inicio" }));
    await waitFor(() => expect(fakePath()).toBe("/"));
  });

  it("with the flag ON paints the catalog", async () => {
    render(
      <Providers flags={{ module_config: true }}>
        <AppShell />
      </Providers>,
    );

    expect(await screen.findByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Módulo «Configuración»" })).toBeNull();
    await waitFor(() => expect(vi.mocked(listCycles)).toHaveBeenCalled());
  });
});

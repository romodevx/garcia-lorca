import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers, fakePath, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { me as apiMe } from "./auth/api";

// Regression: ISSUE-001 — the "Nivel eliminado…" toast never appeared.
// Found by /qa on 2026-09-28
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-28-eliminar-niveles.md
//
// Next's router REMOUNTS <AppShell> on every route change (the page lives under
// the [[...slug]] catch-all), so a notice kept in AppShell's own state died with
// the level-deletion page. This file mounts a FRESH AppShell at /configuracion
// after the deletion, exactly as the browser does.

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
  getSchoolProfile: vi.fn().mockResolvedValue({ profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "", clabe: "" }, found: false }),
  listLevelTrash: vi.fn().mockResolvedValue([]),
  getLevelDeletionPreview: vi.fn().mockResolvedValue({
    levelId: "l1", levelName: "PRIMARIA", empty: true, confirmToken: "tok", expiresAt: "2026-09-28T18:00:00Z",
    counts: { gradeLevels: 0, groups: 0, subjects: 0, terms: 0, teachingAssignments: 0, students: 0, enrollments: 0, attendanceMarks: 0, monthlyGrades: 0, reportCards: 0, tuitionPlans: 0, uniformItems: 0, schoolEvents: 0 },
    keeps: { charges: 0, payments: 0 },
  }),
  deleteLevel: vi.fn().mockResolvedValue({ batchId: "b1", deletedAt: "2026-09-28T17:00:00Z", restorableUntil: "2026-10-05T17:00:00Z", counts: {} }),
}));

vi.mock("./finance/api", () => ({
  listTuitionPlans: vi.fn().mockResolvedValue([]),
}));

const DIRECTOR_ME = {
  userId: "u1",
  email: "owner@example.com",
  permissions: ["config:read", "config:write", "levels:delete"],
};

const DONE = "Nivel eliminado. Puedes restaurarlo desde la Papelera hasta el 5 de octubre de 2026.";

function mountShell(): ReturnType<typeof render> {
  return render(
    <Providers flags={{ module_config: true }}>
      <AppShell />
    </Providers>
  );
}

beforeEach(() => {
  resetFakeRouter("/configuracion/niveles/l1/eliminar");
  vi.mocked(apiMe).mockResolvedValue(DIRECTOR_ME);
});

afterEach(() => vi.clearAllMocks());

describe("el aviso de un nivel eliminado sobrevive al cambio de ruta", () => {
  it("una AppShell nueva en /configuracion muestra el aviso", async () => {
    const user = userEvent.setup();
    const first = mountShell();

    await user.click(await screen.findByRole("button", { name: "Continuar" }));
    await user.type(screen.getByLabelText("Escribe el nombre del nivel para confirmar: PRIMARIA"), "primaria");
    await user.click(screen.getByRole("button", { name: "Continuar" }));
    await user.type(screen.getByLabelText("Tu contraseña de director"), "secreto");
    await user.click(screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" }));
    await screen.findByRole("heading", { name: "Configuración académica" });
    expect(fakePath()).toBe("/configuracion");

    // What Next does on a route change: the page's tree is a new one.
    first.unmount();
    mountShell();

    expect(await screen.findByText(DONE)).toBeInTheDocument();
  });

  it("el aviso se da una sola vez: volver a Configuración después ya no lo repite", async () => {
    const user = userEvent.setup();
    const first = mountShell();

    await user.click(await screen.findByRole("button", { name: "Continuar" }));
    await user.type(screen.getByLabelText("Escribe el nombre del nivel para confirmar: PRIMARIA"), "primaria");
    await user.click(screen.getByRole("button", { name: "Continuar" }));
    await user.type(screen.getByLabelText("Tu contraseña de director"), "secreto");
    await user.click(screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" }));
    await screen.findByRole("heading", { name: "Configuración académica" });

    first.unmount();
    const second = mountShell();
    await screen.findByText(DONE);
    second.unmount();

    mountShell();
    await screen.findByRole("heading", { name: "Configuración académica" });
    expect(screen.queryByText(DONE)).toBeNull();
  });
});

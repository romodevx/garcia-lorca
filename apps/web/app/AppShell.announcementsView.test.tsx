import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { me as apiMe } from "./auth/api";

/*
 * module_announcements_view picks the view of /avisos (PLAN-redesign-avisos §1,
 * E16): read once here, inside the module_comms branch, never by the screen.
 * comms OFF keeps the placeholder whatever the view flag says.
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

vi.mock("./students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));

const NEW_VIEW_ONLY = "Destinatarios";
const OLD_VIEW_ONLY = "¿A quién?";

beforeEach(() => {
  resetFakeRouter("/avisos");
  vi.mocked(apiMe).mockResolvedValue({ userId: "u1", email: "owner@example.com", permissions: ["comms:read", "comms:write"] });
});
afterEach(() => vi.clearAllMocks());

describe("/avisos and the module_announcements_view flag", () => {
  it("with comms ON and the view OFF paints today's screen", async () => {
    render(
      <Providers flags={{ module_comms: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByText(OLD_VIEW_ONLY)).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: NEW_VIEW_ONLY })).toBeNull();
  });

  it("with comms ON and the view ON paints Destinatarios al lado", async () => {
    render(
      <Providers flags={{ module_comms: true, module_announcements_view: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByRole("heading", { name: NEW_VIEW_ONLY })).toBeInTheDocument();
    expect(screen.queryByText(OLD_VIEW_ONLY)).toBeNull();
  });

  it("with comms OFF paints the placeholder even with the view ON", async () => {
    render(
      <Providers flags={{ module_announcements_view: true }}>
        <AppShell />
      </Providers>,
    );
    expect(await screen.findByRole("heading", { level: 1, name: "Avisos" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: NEW_VIEW_ONLY })).toBeNull();
    expect(screen.queryByText(OLD_VIEW_ONLY)).toBeNull();
    expect(screen.queryByRole("button", { name: /Enviar aviso/ })).toBeNull();
  });
});

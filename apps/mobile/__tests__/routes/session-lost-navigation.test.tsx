// The REAL router and the REAL route tree under app/: the bug lived in how
// expo-router resolves an href, which the global router mock cannot see.
jest.unmock("expo-router");
// Push registration needs the native module; it is not what this test is about.
jest.mock("@/src/push/usePushNotifications", () => ({ usePushNotifications: () => undefined }));

import { act, cleanup, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { renderRouter } from "expo-router/testing-library";
import { resolve } from "node:path";
import { clearTokens, getRefreshToken, saveTokens } from "@/src/api";
import { RETURN_TO_PARAM } from "@/src/navigation/routes";

const APP_DIR = resolve(__dirname, "../../app");
const SESSION_ENDED = "Tu sesión terminó. Vuelve a entrar.";

type FetchAnswer = { ok: boolean; status: number; json: () => Promise<unknown> };

const answer = (status: number, body: unknown = {}): FetchAnswer => ({
  ok: status >= 200 && status < 300,
  status,
  json: async () => body,
});

const KID = { id: "s1", firstName: "Alumna", lastName: "Prueba", level: "Primaria", levelId: "l1", gradeGroup: "B" };

/** A gateway that revoked this phone's refresh token: every call is 401. */
function revokedGateway(): jest.Mock {
  return jest.fn(async () => answer(401));
}

/** A gateway that is unreachable: every call fails at the network layer. */
function unreachableGateway(): jest.Mock {
  return jest.fn(async () => {
    throw new TypeError("Network request failed");
  });
}

/** A healthy gateway that knows one child, so the tabs render real data. */
function healthyGateway(): jest.Mock {
  return jest.fn(async (url: string) => {
    const u = String(url);
    if (u.endsWith("/family/students")) return answer(200, { students: [KID] });
    if (u.includes("/statement")) {
      return answer(200, {
        movements: [],
        totals: { chargedCents: 0, discountCents: 0, paidCents: 0, balanceCents: 0, notYetDueCents: 0 },
        dueTodayCents: 0,
        asOf: "2026-10-17",
      });
    }
    return answer(200, { notifications: [], students: [], days: [] });
  });
}

describe("root navigation when the stored session dies (ISSUE-T14-102)", () => {
  afterEach(async () => {
    cleanup();
    await clearTokens();
  });

  it("returns to Entrar with a notice when the refresh is rejected while inside the tabs", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    globalThis.fetch = healthyGateway() as unknown as typeof fetch;
    await renderRouter(APP_DIR, { initialUrl: "/cuenta" });
    await waitFor(() => expect(screen.getAllByText("Cuenta").length).toBeGreaterThan(0));

    // The password changed: the access token expires and the refresh is refused.
    globalThis.fetch = revokedGateway() as unknown as typeof fetch;
    await act(async () => {
      router.navigate("/asistencia");
    });

    await waitFor(() => expect(screen.getByText(SESSION_ENDED)).toBeTruthy());
    expect(screen.getByRole("button", { name: "Ingresar" })).toBeTruthy();
    expect(getRefreshToken()).toBeNull();
  });

  it("does not remember the dead session's route, so the next guardian starts clean (bug_006)", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    globalThis.fetch = healthyGateway() as unknown as typeof fetch;
    // With RNTL 14 render is async, so renderRouter's route getters hang off the
    // promise it returns, not off the awaited result.
    const app = renderRouter(APP_DIR, { initialUrl: "/cuenta" });
    await app;
    await waitFor(() => expect(screen.getAllByText("Cuenta").length).toBeGreaterThan(0));

    globalThis.fetch = revokedGateway() as unknown as typeof fetch;
    await act(async () => {
      router.navigate("/asistencia");
    });

    await waitFor(() => expect(screen.getByText(SESSION_ENDED)).toBeTruthy());
    expect(app.getPathname()).toBe("/");
    expect(app.getSearchParams()).not.toHaveProperty(RETURN_TO_PARAM);
  });

  it("opens on Entrar with the notice when a cold start finds a dead stored session", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    globalThis.fetch = revokedGateway() as unknown as typeof fetch;

    await renderRouter(APP_DIR, { initialUrl: "/" });

    await waitFor(() => expect(screen.getByText(SESSION_ENDED)).toBeTruthy());
    expect(screen.getByRole("button", { name: "Ingresar" })).toBeTruthy();
  });

  it("keeps the family signed in and says 'Revisa tu conexión' when the network is down", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    globalThis.fetch = unreachableGateway() as unknown as typeof fetch;

    await renderRouter(APP_DIR, { initialUrl: "/" });

    await waitFor(() => expect(screen.getAllByText(/Revisa tu conexión/).length).toBeGreaterThan(0));
    expect(screen.queryByText(SESSION_ENDED)).toBeNull();
    expect(screen.queryByRole("button", { name: "Ingresar" })).toBeNull();
    expect(getRefreshToken()).toBe("r1");
  });
});

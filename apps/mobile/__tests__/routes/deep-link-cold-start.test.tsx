// The REAL router and the REAL route tree under app/: what is under test is
// where a cold start lands, which the global router mock cannot see.
jest.unmock("expo-router");
// Push registration needs the native module; it is not what this test is about.
jest.mock("@/src/push/usePushNotifications", () => ({ usePushNotifications: () => undefined }));

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { renderRouter } from "expo-router/testing-library";
import { resolve } from "node:path";
import { clearTokens, getAccessToken, saveTokens } from "@/src/api";

const APP_DIR = resolve(__dirname, "../../app");

const KID = { id: "s1", firstName: "Alumna", lastName: "Prueba", level: "Primaria", levelId: "l1", gradeGroup: "B" };
const NOTICE = {
  id: "n1",
  studentId: "s1",
  type: "announcement",
  title: "Junta de padres el viernes",
  body: "A las 8:00 en el patio.",
  createdAt: "2026-10-21T15:00:00Z",
  readAt: "",
};

type FetchAnswer = { ok: boolean; status: number; json: () => Promise<unknown> };

const answer = (status: number, body: unknown = {}): FetchAnswer => ({
  ok: status >= 200 && status < 300,
  status,
  json: async () => body,
});

/** A healthy gateway that accepts the family's password and knows one notice. */
function healthyGateway(): jest.Mock {
  return jest.fn(async (url: string) => {
    const u = String(url);
    if (u.endsWith("/auth/mobile/login")) {
      return answer(200, { accessToken: "a1", refreshToken: "r1", expiresIn: 900, mustChangePassword: false });
    }
    if (u.endsWith("/family/students")) return answer(200, { students: [KID] });
    if (u.endsWith("/family/notifications")) return answer(200, { notifications: [NOTICE] });
    return answer(200, { notifications: [], students: [], days: [] });
  });
}

describe("a cold start opened by a deep link", () => {
  beforeEach(() => {
    globalThis.fetch = healthyGateway() as unknown as typeof fetch;
  });
  afterEach(async () => {
    cleanup();
    await clearTokens();
  });

  it("opens the linked route when the family already has a session", async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });

    await renderRouter(APP_DIR, { initialUrl: "/avisos" });

    await waitFor(() => expect(screen.getByText(NOTICE.title)).toBeTruthy());
    expect(screen.queryByRole("button", { name: "Ingresar" })).toBeNull();
  });

  it("asks a family without a session to sign in instead of showing the route empty", async () => {
    await renderRouter(APP_DIR, { initialUrl: "/avisos" });

    await waitFor(() => expect(screen.getByRole("button", { name: "Ingresar" })).toBeTruthy());
    expect(screen.queryByText("No hay avisos nuevos")).toBeNull();
  });

  it("takes the family to the linked route once they sign in", async () => {
    await renderRouter(APP_DIR, { initialUrl: "/avisos" });
    await waitFor(() => expect(screen.getByRole("button", { name: "Ingresar" })).toBeTruthy());

    fireEvent.changeText(screen.getByLabelText("Correo electrónico"), "familia@example.test");
    fireEvent.changeText(screen.getByLabelText("Contraseña"), "secreta");
    fireEvent.press(screen.getByRole("button", { name: "Ingresar" }));

    await waitFor(() => expect(screen.getByText(NOTICE.title)).toBeTruthy());
    expect(getAccessToken()).toBe("a1");
  });
});

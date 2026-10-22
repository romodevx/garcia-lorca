// The REAL router and the REAL route tree under app/. Kept apart from
// deep-link-cold-start.test.tsx: expo-router's store is module state, so a
// second sign-in through the UI in the same file inherits the first one's route.
jest.unmock("expo-router");
// Push registration needs the native module; it is not what this test is about.
jest.mock("@/src/push/usePushNotifications", () => ({ usePushNotifications: () => undefined }));

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { renderRouter } from "expo-router/testing-library";
import { resolve } from "node:path";
import { clearTokens } from "@/src/api";

const APP_DIR = resolve(__dirname, "../../app");

const KID = { id: "s1", firstName: "Alumna", lastName: "Prueba", level: "Primaria", levelId: "l1", gradeGroup: "B" };
// The link names the SECOND child: without its query the boleta would fall
// back to the selected (first) child and still look right.
const SIBLING = { id: "s2", firstName: "Hermano", lastName: "Prueba", level: "Primaria", levelId: "l1", gradeGroup: "C" };

type FetchAnswer = { ok: boolean; status: number; json: () => Promise<unknown> };

const answer = (status: number, body: unknown = {}): FetchAnswer => ({
  ok: status >= 200 && status < 300,
  status,
  json: async () => body,
});

/** A healthy gateway that accepts the family's password; the boleta itself is not under test. */
function healthyGateway(): jest.Mock {
  return jest.fn(async (url: string) => {
    const u = String(url);
    if (u.endsWith("/auth/mobile/login")) {
      return answer(200, { accessToken: "a1", refreshToken: "r1", expiresIn: 900, mustChangePassword: false });
    }
    if (u.endsWith("/family/students")) return answer(200, { students: [KID, SIBLING] });
    if (u.includes("/report-cards")) return answer(404);
    return answer(200, { notifications: [], students: [], days: [] });
  });
}

describe("a cold start opened by a deep link with a query", () => {
  afterEach(async () => {
    cleanup();
    await clearTokens();
  });

  it("keeps the link's query through the login, so a boleta opens for the student it names", async () => {
    globalThis.fetch = healthyGateway() as unknown as typeof fetch;
    // With RNTL 14 render is async, so renderRouter's route getters hang off the
    // promise it returns, not off the awaited result.
    const app = renderRouter(APP_DIR, { initialUrl: "/boleta/r1?studentId=s2" });
    await app;
    await waitFor(() => expect(screen.getByRole("button", { name: "Ingresar" })).toBeTruthy());

    fireEvent.changeText(screen.getByLabelText("Correo electrónico"), "familia@example.test");
    fireEvent.changeText(screen.getByLabelText("Contraseña"), "secreta");
    fireEvent.press(screen.getByRole("button", { name: "Ingresar" }));

    await waitFor(() => expect(app.getPathname()).toBe("/boleta/r1"));
    expect(app.getSearchParams()).toEqual({ id: "r1", studentId: "s2" });
  });
});

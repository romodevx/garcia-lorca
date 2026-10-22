import AsyncStorage from "@react-native-async-storage/async-storage";
import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { type ReactNode } from "react";
import { clearTokens, saveTokens } from "@/src/api";
import { AppHeader } from "@/src/components/AppHeader";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import CuentaRoute from "../../app/(tabs)/cuenta";
import InicioRoute from "../../app/(tabs)/inicio";

const CUATRO = { id: "s4", firstName: "ALUMNA", lastName: "CUATRO", level: "Primaria", levelId: "l1", gradeGroup: "B" };
const DOS = { id: "s2", firstName: "ALUMNA", lastName: "DOS", level: "Primaria", levelId: "l1", gradeGroup: "A" };

// QA 21-oct: after switching child, Cuenta opened at the previous child's
// scroll offset (the bottom of their ledger). Each child's screen starts at
// the top: the scroll container is a new one per child.
describe.each<[string, () => ReactNode]>([
  ["Cuenta", () => <CuentaRoute />],
  ["Inicio", () => <InicioRoute />],
])("%s after switching child", (_name, Route) => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    globalThis.fetch = jest.fn(async (url: string) => {
      const body = String(url).endsWith("/family/students") ? { students: [CUATRO, DOS] } : { notifications: [] };
      return { ok: true, status: 200, json: async () => body };
    }) as unknown as typeof fetch;
  });
  afterEach(async () => {
    await clearTokens();
    await AsyncStorage.clear();
  });

  it("starts from a fresh scroll container, not the previous child's offset", async () => {
    await renderWithProviders(
      <>
        <AppHeader />
        <Route />
      </>,
      { flags: { module_finance: true } },
    );
    await screen.findByRole("button", { name: /Ahora: Alumna Cuatro/ });
    const before = screen.getByTestId("screen-scroll");

    await fireEvent.press(screen.getByRole("button", { name: /Cambiar de hijo/ }));
    await fireEvent.press(await screen.findByRole("radio", { name: /Alumna Dos/ }));

    await waitFor(() => expect(screen.getByRole("button", { name: /Ahora: Alumna Dos/ })).toBeTruthy());
    expect(screen.getByTestId("screen-scroll")).not.toBe(before);
  });
});

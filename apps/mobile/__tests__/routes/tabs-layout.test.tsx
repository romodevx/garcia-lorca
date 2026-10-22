import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { type ReactNode } from "react";
import { clearTokens, saveTokens } from "@/src/api";
import { demoFlags } from "@/src/flags/bootstrap";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import TabsLayout from "../../app/(tabs)/_layout";

// The navigator itself is not under test: a stand-in renders no scenes, only
// the layout's own tab bar, so what remains is the chrome around every tab.
jest.mock("expo-router", () => {
  const barProps = { state: { index: 0, routes: [{ key: "inicio", name: "inicio" }] }, navigation: { navigate: jest.fn() } };
  const Tabs = Object.assign(
    ({ tabBar }: { tabBar: (props: unknown) => ReactNode; children?: ReactNode }) => tabBar(barProps),
    { Screen: () => null },
  );
  return { Tabs, router: { push: jest.fn(), replace: jest.fn(), back: jest.fn(), navigate: jest.fn() } };
});

const ZOE = { id: "s1", firstName: "Zoe", lastName: "Villalobos", level: "Primaria", gradeGroup: "1° A" };

describe("TabsLayout", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("mounts the card button over every tab", async () => {
    globalThis.fetch = jest.fn(async (url: string) => {
      const body = String(url).endsWith("/family/students") ? { students: [ZOE] } : { notifications: [] };
      return { ok: true, status: 200, json: async () => body };
    }) as unknown as typeof fetch;

    await renderWithProviders(<TabsLayout />, { flags: { ...demoFlags, module_student_card: true } });
    await waitFor(() => expect(screen.getByText("Zoe Villalobos")).toBeTruthy());
    expect(screen.queryByRole("button", { name: "Mostrar credencial" })).toBeNull();

    fireEvent(screen.getByTestId("tab-bar"), "layout", { nativeEvent: { layout: { x: 0, y: 0, width: 390, height: 83 } } });

    expect(await screen.findByRole("button", { name: "Mostrar credencial" })).toHaveStyle({ bottom: 99 });
  });
});

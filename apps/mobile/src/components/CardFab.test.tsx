import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { clearTokens, saveTokens } from "../api";
import { demoFlags } from "../flags/bootstrap";
import { lightColors } from "../theme/tokens";
import { renderWithProviders } from "../test/renderWithProviders";
import { CardFab } from "./CardFab";

const ZOE = { id: "s1", firstName: "Zoe", lastName: "Villalobos", level: "Primaria", gradeGroup: "1° A" };
const cardOn = { ...demoFlags, module_student_card: true };

function mockChildren(students: unknown[]): void {
  globalThis.fetch = jest.fn(async (url: string) => {
    const body = String(url).endsWith("/family/students") ? { students } : { notifications: [] };
    return { ok: true, status: 200, json: async () => body };
  }) as unknown as typeof fetch;
}

const childrenRequested = (): boolean =>
  (globalThis.fetch as jest.Mock).mock.calls.some((c) => String((c as unknown[])[0]).endsWith("/family/students"));

describe("CardFab", () => {
  beforeEach(async () => {
    (router.push as jest.Mock).mockClear();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("opens the selected child's card", async () => {
    mockChildren([ZOE]);
    await renderWithProviders(<CardFab bottom={83} />, { flags: cardOn });

    await fireEvent.press(await screen.findByRole("button", { name: "Mostrar credencial" }));

    expect(router.push).toHaveBeenCalledWith("/credencial");
  });

  // The bar is measured after the first frame; until then the button would sit on it.
  it("waits for the tab bar to be measured", async () => {
    mockChildren([ZOE]);
    await renderWithProviders(<CardFab bottom={0} />, { flags: cardOn });

    await waitFor(() => expect(childrenRequested()).toBe(true));
    expect(screen.queryByRole("button", { name: "Mostrar credencial" })).toBeNull();
  });

  it("floats 16 above the tab bar, on the primary color", async () => {
    mockChildren([ZOE]);
    await renderWithProviders(<CardFab bottom={83} />, { flags: cardOn });

    const fab = await screen.findByRole("button", { name: "Mostrar credencial" });
    expect(fab).toHaveStyle({ bottom: 99, backgroundColor: lightColors.primary });
  });

  it("is absent while the card module is off", async () => {
    mockChildren([ZOE]);
    await renderWithProviders(<CardFab bottom={83} />, { flags: { ...demoFlags, module_student_card: false } });

    await waitFor(() => expect(childrenRequested()).toBe(true));
    expect(screen.queryByRole("button", { name: "Mostrar credencial" })).toBeNull();
  });

  it("is absent when there is no child to show", async () => {
    mockChildren([]);
    await renderWithProviders(<CardFab bottom={83} />, { flags: cardOn });

    await waitFor(() => expect(childrenRequested()).toBe(true));
    expect(screen.queryByRole("button", { name: "Mostrar credencial" })).toBeNull();
  });
});

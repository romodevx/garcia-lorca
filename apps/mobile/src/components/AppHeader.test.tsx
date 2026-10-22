import AsyncStorage from "@react-native-async-storage/async-storage";
import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { AccountScreen } from "../screens/AccountScreen";
import { renderWithProviders } from "../test/renderWithProviders";
import { AppHeader } from "./AppHeader";

const CUATRO = { id: "s4", firstName: "ALUMNA", lastName: "CUATRO", level: "Primaria", levelId: "l1", gradeGroup: "B", groupLabel: "3° PRIMARIA · B" };
const DOS = { id: "s2", firstName: "ALUMNA", lastName: "DOS", level: "Primaria", levelId: "l1", gradeGroup: "A", groupLabel: "1° PRIMARIA · A" };

function mockChildren(students: unknown[]): void {
  globalThis.fetch = jest.fn(async (url: string) => {
    const body = String(url).endsWith("/family/students") ? { students } : { notifications: [] };
    return { ok: true, status: 200, json: async () => body };
  }) as unknown as typeof fetch;
}

// ISSUE-CC-303: a family with two children only ever saw the first.
describe("AppHeader child switcher", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
    await AsyncStorage.clear();
  });

  it("offers to switch child when the family has more than one, and switches", async () => {
    mockChildren([CUATRO, DOS]);
    await renderWithProviders(<AppHeader />);

    const switcher = await screen.findByRole("button", { name: "Cambiar de hijo(a). Ahora: Alumna Cuatro" });
    await fireEvent.press(switcher);
    await fireEvent.press(await screen.findByRole("radio", { name: "Alumna Dos, 1° PRIMARIA · A" }));

    await waitFor(() => expect(screen.getByRole("button", { name: "Cambiar de hijo(a). Ahora: Alumna Dos" })).toBeTruthy());
    expect(screen.getByText("1° PRIMARIA · A")).toBeTruthy();
  });

  it("moves the per-child screens to the chosen child", async () => {
    mockChildren([CUATRO, DOS]);
    await renderWithProviders(
      <>
        <AppHeader />
        <AccountScreen />
      </>,
    );
    const calledFor = (id: string): boolean =>
      (globalThis.fetch as jest.Mock).mock.calls.some((c) => String((c as unknown[])[0]).includes(`/family/students/${id}/statement`));
    await waitFor(() => expect(calledFor("s4")).toBe(true));
    expect(calledFor("s2")).toBe(false);

    await fireEvent.press(await screen.findByRole("button", { name: /Cambiar de hijo/ }));
    await fireEvent.press(await screen.findByRole("radio", { name: /Alumna Dos/ }));

    await waitFor(() => expect(calledFor("s2")).toBe(true));
  });

  it("has no switcher for a single child", async () => {
    mockChildren([CUATRO]);
    await renderWithProviders(<AppHeader />);

    await waitFor(() => expect(screen.getByText("Alumna Cuatro")).toBeTruthy());
    expect(screen.queryByRole("button", { name: /Cambiar de hijo/ })).toBeNull();
  });
});

// QA 21-oct: switching to EN changed only the toggle's own label — every screen
// is still written in Spanish. A control that does nothing visible is a defect.
describe("AppHeader language", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
    await AsyncStorage.clear();
  });

  it("offers no language switch until the screens are translated", async () => {
    mockChildren([CUATRO]);
    await renderWithProviders(<AppHeader />);

    await waitFor(() => expect(screen.getByText("Alumna Cuatro")).toBeTruthy());
    expect(screen.queryByText("EN")).toBeNull();
    expect(screen.getAllByRole("button").map((b) => b.props.accessibilityLabel)).toEqual(["Cambiar tema", "Avisos"]);
  });
});

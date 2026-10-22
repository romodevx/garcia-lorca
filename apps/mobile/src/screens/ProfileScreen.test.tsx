import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { ProfileScreen } from "./ProfileScreen";
import { renderWithProviders } from "../test/renderWithProviders";

const child = { id: "s1", firstName: "Mateo", lastName: "Herrera", maternalLastName: "Díaz", level: "Primaria", levelId: "l1", gradeGroup: "A" };
const enrollment = { id: "e1", studentId: "s1", cycleId: "c1", groupId: "g1", termId: "t1", status: "active", enrolledAt: "2026-08-24" };

function mockFetch(profile: unknown | null): void {
  globalThis.fetch = jest.fn(async (url: string) => {
    const u = String(url);
    if (u.includes("/family/students/s1/profile")) {
      if (profile === null) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => profile };
    }
    if (u.includes("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [child] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  }) as unknown as typeof fetch;
}

// QA 21-oct: "Perfil del alumno" in Más did nothing when tapped.
describe("ProfileScreen", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("shows the child's name, level, group and current enrollment", async () => {
    mockFetch({ ...child, groupLabel: "3° PRIMARIA · A", currentEnrollment: enrollment });

    await renderWithProviders(<ProfileScreen />);

    await waitFor(() => expect(screen.getByText("Herrera Díaz Mateo")).toBeTruthy());
    expect(screen.getByText("Primaria")).toBeTruthy();
    expect(screen.getByText("3° PRIMARIA · A")).toBeTruthy();
    expect(screen.getByText("Inscrito")).toBeTruthy();
    expect(screen.getByText("Desde el 24 ago 2026")).toBeTruthy();
  });

  it("says so when the child has no current enrollment", async () => {
    mockFetch({ ...child, groupLabel: "A", currentEnrollment: null });

    await renderWithProviders(<ProfileScreen />);

    await waitFor(() => expect(screen.getByText("Sin inscripción vigente")).toBeTruthy());
    expect(screen.queryByText("Inscrito")).toBeNull();
  });

  it("offers a retry when the profile cannot be read", async () => {
    mockFetch(null);

    await renderWithProviders(<ProfileScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar el perfil")).toBeTruthy());
    mockFetch({ ...child, groupLabel: "A", currentEnrollment: enrollment });
    await act(async () => {
      fireEvent.press(screen.getByRole("button", { name: "Reintentar" }));
    });
    await waitFor(() => expect(screen.getByText("Herrera Díaz Mateo")).toBeTruthy());
  });
});

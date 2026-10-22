import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { ProfileScreen } from "./ProfileScreen";
import { renderWithProviders } from "../test/renderWithProviders";

const child = { id: "s1", firstName: "Mateo", lastName: "Herrera", maternalLastName: "Díaz", level: "Primaria", levelId: "l1", gradeGroup: "A" };
const enrollment = { id: "e1", studentId: "s1", cycleId: "c1", groupId: "g1", termId: "t1", status: "active", enrolledAt: "2026-08-24" };

function mockFetch(profile: unknown | null, students: unknown[] = [child]): void {
  globalThis.fetch = jest.fn(async (url: string) => {
    const u = String(url);
    if (u.includes("/family/students/s1/profile")) {
      if (profile === null) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => profile };
    }
    if (u.includes("/family/students")) return { ok: true, status: 200, json: async () => ({ students }) };
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

  // The school's admission status (e.g. "Trámite" while the inscription is
  // unpaid) is not on the family profile; an "Inscrito" chip read from the
  // group placement contradicted it (tech-lead review). Only the placement shows.
  it("shows the child's name, level, group and since when they are in it, with no admission chip", async () => {
    mockFetch({ ...child, groupLabel: "3° PRIMARIA · A", currentEnrollment: enrollment });

    await renderWithProviders(<ProfileScreen />);

    await waitFor(() => expect(screen.getByText("Herrera Díaz Mateo")).toBeTruthy());
    expect(screen.getByText("Primaria")).toBeTruthy();
    expect(screen.getByText("3° PRIMARIA · A")).toBeTruthy();
    expect(screen.getByText("En el grupo desde el 24 ago 2026")).toBeTruthy();
    expect(screen.queryByText("Inscrito")).toBeNull();
  });

  it("says so when the child is in no group this cycle", async () => {
    mockFetch({ ...child, groupLabel: "A", currentEnrollment: null });

    await renderWithProviders(<ProfileScreen />);

    await waitFor(() => expect(screen.getByText("Sin grupo asignado")).toBeTruthy());
    expect(screen.queryByText(/En el grupo desde/)).toBeNull();
  });

  it("shows an empty state, not a blank page, when the family has no linked child", async () => {
    mockFetch(null, []);

    await renderWithProviders(<ProfileScreen />);

    await waitFor(() => expect(screen.getByText("Sin alumno vinculado")).toBeTruthy());
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

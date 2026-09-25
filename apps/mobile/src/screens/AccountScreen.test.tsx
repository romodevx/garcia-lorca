import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { demoFlags } from "../flags/bootstrap";
import { clearTokens, saveTokens } from "../api";
import { AccountScreen } from "./AccountScreen";
import { renderWithProviders } from "../test/renderWithProviders";

const child = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "primaria", gradeGroup: "3° A" };

/**
 * Routes each family request to a body by path so the child bootstrap and the
 * account read both resolve. `account` null makes the account endpoint fail.
 */
function mockFetch(account: unknown | null): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    if (u.includes("/family/students/s1/account")) {
      if (account === null) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => account };
    }
    if (u.includes("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [child] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

describe("AccountScreen (real family account)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("renders the real balance and a charge from the account endpoint", async () => {
    mockFetch({
      charges: [
        {
          id: "c1",
          description: "Colegiatura junio",
          amountCents: 240000,
          discountCents: 0,
          paidCents: 0,
          remainingCents: 240000,
          dueDate: "2026-06-15",
          status: "pending",
        },
      ],
      payments: [{ id: "p1", chargeId: "c0", amountCents: 240000, method: "card", reference: "", paidAt: "2026-06-02T10:00:00Z" }],
      totalBalanceCents: 240000,
    });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Colegiatura junio")).toBeTruthy());
    expect(screen.getByLabelText("Saldo").props.children).toBe("$2,400.00");
    expect(screen.getByText("Pago recibido")).toBeTruthy();
  });

  // Task 13: with the card's flag on, Cuenta leads to the child's card; off,
  // nothing hints that it exists.
  it("offers the card only with its flag on", async () => {
    mockFetch({ charges: [], payments: [], totalBalanceCents: 0 });
    await renderWithProviders(<AccountScreen />, { flags: { ...demoFlags, module_student_card: true } });

    fireEvent.press(await screen.findByLabelText("Credencial"));
    expect(router.push).toHaveBeenCalledWith("/credencial");
  });

  it("hides the card while its flag is off", async () => {
    mockFetch({ charges: [], payments: [], totalBalanceCents: 0 });
    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Al corriente")).toBeTruthy());
    expect(screen.queryByLabelText("Credencial")).toBeNull();
  });

  it("shows the settled state when the balance is zero", async () => {
    mockFetch({ charges: [], payments: [{ id: "p1", chargeId: "c0", amountCents: 100, method: "card", reference: "", paidAt: "2026-06-02T10:00:00Z" }], totalBalanceCents: 0 });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Al corriente")).toBeTruthy());
    expect(screen.getByText("$0.00")).toBeTruthy();
  });

  it("shows an empty state when there are no movements", async () => {
    mockFetch({ charges: [], payments: [], totalBalanceCents: 0 });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Sin movimientos")).toBeTruthy());
  });

  it("shows an error state with retry when the account load fails", async () => {
    mockFetch(null);

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar tu cuenta")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
  });

  it("shows the error state (not a fabricated settled balance) when the child bootstrap fails (bug_005)", async () => {
    // The child list load fails, so childError is true and selectedChildId is null.
    // The screen must surface the error, never the fabricated "Al corriente / $0.00".
    const fetchMock = jest.fn(async (url: string) => {
      const u = String(url);
      if (u.includes("/family/students")) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar tu cuenta")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
    expect(screen.queryByText("Al corriente")).toBeNull();
    expect(screen.queryByText("$0.00")).toBeNull();
  });
});

describe("AccountScreen — lo que se debe hoy", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  // The parent is the one person who cannot walk to the office and ask what is
  // owed. The hero must lead with what the school is asking for TODAY, and keep
  // the year's balance beside it rather than instead of it.
  it("encabeza con lo del día y deja el año al lado", async () => {
    mockFetch({
      charges: [],
      payments: [],
      totalBalanceCents: 3200000,
      dueTodayCents: 775000,
      surchargeCents: 15000,
      earlyDiscountCents: 10000,
      asOf: "2026-11-20",
    });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByLabelText("Saldo").props.children).toBe("$7,750.00"));
    expect(screen.getByLabelText("Saldo del año").props.children).toBe("$32,000.00");
  });

  // A family asking "why that number?" has no counter to ask it at.
  it("explica el recargo, el pronto pago y el día", async () => {
    mockFetch({
      charges: [],
      payments: [],
      totalBalanceCents: 3200000,
      dueTodayCents: 775000,
      surchargeCents: 15000,
      earlyDiscountCents: 10000,
      asOf: "2026-11-20",
    });

    await renderWithProviders(<AccountScreen />);

    const note = await screen.findByLabelText("Detalle del saldo");
    expect(String(note.props.children)).toContain("150.00");
    expect(String(note.props.children)).toContain("100.00");
    expect(String(note.props.children)).toContain("2026-11-20");
  });

  // A gateway older than M18 sends no asOf. Showing the day's figure as $0.00
  // would tell a family that owes $32,000 that they are settled — the exact
  // fabricated "Al corriente" this screen already refuses elsewhere (bug_005).
  it("no inventa un saldo del día cuando el servidor no lo mandó", async () => {
    mockFetch({ charges: [], payments: [], totalBalanceCents: 3200000 });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByLabelText("Saldo").props.children).toBe("$32,000.00"));
    expect(screen.queryByLabelText("Saldo del año")).toBeNull();
    expect(screen.queryByText("Al corriente")).toBeNull();
  });
});

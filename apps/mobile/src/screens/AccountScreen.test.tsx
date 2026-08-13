import { screen, waitFor } from "@testing-library/react-native";
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
});

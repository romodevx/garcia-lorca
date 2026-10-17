import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { demoFlags } from "../flags/bootstrap";
import { clearTokens, saveTokens } from "../api";
import { downloadPdf, PdfError, sharePdf } from "../files/reportCardPdf";
import { AccountScreen } from "./AccountScreen";
import { renderWithProviders } from "../test/renderWithProviders";

jest.mock("../files/reportCardPdf", () => {
  const actual = jest.requireActual("../files/reportCardPdf");
  return { ...actual, downloadPdf: jest.fn(async () => "file:///cache/estado.pdf"), sharePdf: jest.fn(async () => undefined) };
});

const child = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "primaria", gradeGroup: "3° A" };

const ZERO = { chargedCents: 0, discountCents: 0, paidCents: 0, balanceCents: 0, notYetDueCents: 0 };

function line(kind: string, date: string, description: string, amountCents: number, balanceCents: number, extra: object = {}): object {
  return { kind, date, description, amountCents, balanceCents, accrued: false, notYetDue: false, voided: false, ...extra };
}

/**
 * Routes each family request to a body by path so the child bootstrap and the
 * statement read both resolve. `statement` null makes the statement fail.
 */
function mockFetch(statement: unknown | null): jest.Mock {
  const fetchMock = jest.fn(async (url: string) => {
    const u = String(url);
    if (u.includes("/family/students/s1/statement")) {
      if (statement === null) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => statement };
    }
    if (u.includes("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [child] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

describe("AccountScreen (the backend's ledger, T7)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
    jest.clearAllMocks();
  });

  it("renders the ledger as finance sent it, newest first", async () => {
    const fetchMock = mockFetch({
      movements: [
        line("charge", "2026-09-10", "Colegiatura septiembre", 320000, 320000),
        line("payment", "2026-09-12", "Pago", 100000, 220000, { method: "card", folio: "P-2026-000042" }),
        line("surcharge", "2026-10-01", "Recargo por atraso · Colegiatura septiembre", 20000, 240000, { accrued: true }),
      ],
      totals: { ...ZERO, chargedCents: 340000, paidCents: 100000, balanceCents: 240000 },
      dueTodayCents: 240000,
      asOf: "2026-10-01",
    });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Colegiatura septiembre")).toBeTruthy());
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/family/students/s1/statement"))).toBe(true);
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/account"))).toBe(false);
    const rows = screen.getAllByLabelText(/^Movimiento /);
    expect(rows.map((r) => r.props.accessibilityLabel)).toEqual([
      "Movimiento Recargo por atraso · Colegiatura septiembre",
      "Movimiento Pago",
      "Movimiento Colegiatura septiembre",
    ]);
    expect(screen.getByText("Acumulado a hoy")).toBeTruthy();
    expect(screen.getByText(/P-2026-000042/)).toBeTruthy();
  });

  // ISSUE-T14-106: a payment made before its charge's due day (the inscription
  // moved to the 19th, paid on the 17th) gave running balances below zero that
  // read as credit. A per-line balance cannot mean anything once a payment can
  // precede its charge, so the rows carry their amount only.
  it("shows no per-line running balance, so a prepayment never reads as credit", async () => {
    mockFetch({
      movements: [
        line("payment", "2026-10-17", "Pago", 300000, -300000, { method: "cash", folio: "P-2026-000002" }),
        line("charge", "2026-10-19", "Inscripción", 300000, 0),
      ],
      totals: { ...ZERO, chargedCents: 300000, paidCents: 300000, balanceCents: 0 },
      dueTodayCents: 0,
      asOf: "2026-10-17",
    });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Inscripción")).toBeTruthy());
    expect(screen.queryByText("Saldo a favor $3,000.00")).toBeNull();
    expect(screen.queryByText(/^Saldo (-|−)?\$/)).toBeNull();
  });

  // ISSUE-T14-106: the cycle's months are all on the ledger, so newest-first put
  // June of next year on top and today's payments at the bottom. What has
  // happened reads newest first; what is still to come sits below, soonest first.
  it("lists what has happened newest first and the months still to come below, soonest first", async () => {
    mockFetch({
      movements: [
        line("charge", "2026-08-24", "Colegiatura agosto", 280000, 280000),
        line("charge", "2026-09-10", "Colegiatura septiembre", 280000, 560000),
        line("payment", "2026-10-17", "Pago", 400000, 160000, { method: "card" }),
        line("charge", "2026-10-19", "Colegiatura octubre", 280000, 440000),
        line("charge", "2026-11-10", "Colegiatura noviembre", 280000, 720000),
        line("charge", "2027-06-10", "Colegiatura junio", 280000, 1000000),
      ],
      totals: { ...ZERO, chargedCents: 1400000, paidCents: 400000, balanceCents: 1000000 },
      dueTodayCents: 160000,
      asOf: "2026-10-17",
    });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Colegiatura junio")).toBeTruthy());
    expect(screen.getAllByLabelText(/^Movimiento /).map((r) => r.props.accessibilityLabel)).toEqual([
      "Movimiento Pago",
      "Movimiento Colegiatura septiembre",
      "Movimiento Colegiatura agosto",
      "Movimiento Colegiatura octubre",
      "Movimiento Colegiatura noviembre",
      "Movimiento Colegiatura junio",
    ]);
    expect(screen.getByText("Próximos cargos")).toBeTruthy();
  });

  // T18: the family's beca is a line of its own under the month, read as a
  // credit (it lowers the balance), with the scholarship's name.
  it("shows a scholarship as its own credit line", async () => {
    mockFetch({
      movements: [
        line("charge", "2026-09-10", "Colegiatura septiembre", 300000, 300000),
        line("scholarship", "2026-09-10", "Beca Hermanos · Colegiatura septiembre", 150000, 150000),
      ],
      totals: { ...ZERO, chargedCents: 300000, discountCents: 150000, balanceCents: 150000 },
      dueTodayCents: 150000,
      asOf: "2026-09-03",
    });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Beca Hermanos · Colegiatura septiembre")).toBeTruthy());
    expect(screen.getByLabelText("Movimiento Beca Hermanos · Colegiatura septiembre")).toBeTruthy();
    expect(screen.getAllByText(/Beca · /).length).toBeGreaterThan(0);
  });

  // T8: a credit note is its own line, named once — not "Nota de crédito · … · credit_note".
  it("names a credit note in words, once", async () => {
    mockFetch({
      movements: [line("credit_note", "2026-10-12", "Nota de crédito", 50000, 0, { method: "credit_note", folio: "P-2026-000050" })],
      totals: ZERO,
      dueTodayCents: 0,
      asOf: "2026-10-12",
    });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText(/P-2026-000050/)).toBeTruthy());
    const sub = screen.getByText(/P-2026-000050/).props.children as string;
    expect(sub).not.toContain("credit_note");
    expect(sub.match(/Nota de crédito/g)).toHaveLength(1);
  });

  it("leads with today's figure and keeps the pending balance beside it", async () => {
    mockFetch({ movements: [], totals: { ...ZERO, balanceCents: 3200000 }, dueTodayCents: 775000, surchargeCents: 15000, earlyDiscountCents: 10000, asOf: "2026-11-20" });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByLabelText("Saldo").props.children).toBe("$7,750.00"));
    expect(screen.getByText("Saldo total $32,000.00")).toBeTruthy();
    const note = screen.getByLabelText("Detalle del saldo");
    expect(String(note.props.children)).toContain("150.00");
    expect(String(note.props.children)).toContain("100.00");
    // The day in words, not the wire's ISO (T5).
    expect(String(note.props.children)).toContain("Al 20 nov 2026.");
    expect(String(note.props.children)).not.toContain("2026-11-20");
  });

  it("says the family has credit when the balance is below zero", async () => {
    mockFetch({ movements: [], totals: { ...ZERO, balanceCents: -50000 }, dueTodayCents: 0, asOf: "2026-10-01" });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Al corriente")).toBeTruthy());
    expect(screen.getByText("Saldo a favor $500.00")).toBeTruthy();
    expect(screen.queryByText(/-\$/)).toBeNull();
  });

  // Without the day the hero falls back to the balance: a credit is still a
  // credit, not "$0.00" and not "Pendiente" (T5).
  it("shows a credit as credit when the server did not send the day", async () => {
    mockFetch({ movements: [], totals: { ...ZERO, balanceCents: -50000 } });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Saldo a favor")).toBeTruthy());
    expect(screen.getByLabelText("Saldo").props.children).toBe("$500.00");
    expect(screen.getByText("Al corriente")).toBeTruthy();
    expect(screen.queryByText("Pendiente")).toBeNull();
  });

  it("names a balance below zero as credit, once (in the header)", async () => {
    mockFetch({
      movements: [line("payment", "2026-09-12", "Pago", 50000, -50000, { method: "cash" })],
      totals: { ...ZERO, paidCents: 50000, balanceCents: -50000 },
      dueTodayCents: 0,
      asOf: "2026-10-01",
    });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Saldo a favor $500.00")).toBeTruthy());
    expect(screen.queryByText(/Saldo -/)).toBeNull();
  });

  it("downloads the statement's PDF and opens the share sheet", async () => {
    mockFetch({ movements: [line("charge", "2026-09-10", "Colegiatura septiembre", 320000, 320000)], totals: ZERO, dueTodayCents: 0, asOf: "2026-10-01" });

    await renderWithProviders(<AccountScreen />);

    fireEvent.press(await screen.findByLabelText("Descargar estado de cuenta"));
    await waitFor(() =>
      expect(downloadPdf).toHaveBeenCalledWith(expect.objectContaining({ path: "/api/v1/family/students/s1/statement/pdf", studentId: "s1" })),
    );
    await waitFor(() => expect(sharePdf).toHaveBeenCalledWith("file:///cache/estado.pdf", "Estado de cuenta"));
  });

  // ISSUE-T14-107: a payment opens its receipt, as the PDF the family shares.
  it("opens a payment's receipt from its row", async () => {
    mockFetch({
      movements: [line("payment", "2026-10-17", "Pago · Inscripción", 300000, 0, { method: "cash", folio: "P-2026-000002", depositId: "dep-1" })],
      totals: ZERO,
      dueTodayCents: 0,
      asOf: "2026-10-17",
    });

    await renderWithProviders(<AccountScreen />);

    fireEvent.press(await screen.findByRole("button", { name: "Movimiento Pago · Inscripción" }));
    await waitFor(() =>
      expect(downloadPdf).toHaveBeenCalledWith(
        expect.objectContaining({ path: "/api/v1/family/students/s1/receipts/dep-1/pdf", studentId: "s1", fallbackName: "comprobante-P-2026-000002.pdf" }),
      ),
    );
    await waitFor(() => expect(sharePdf).toHaveBeenCalledWith("file:///cache/estado.pdf", "Comprobante de pago"));
  });

  it("says in words why a receipt could not be opened", async () => {
    (downloadPdf as jest.Mock).mockRejectedValueOnce(new PdfError("not_found"));
    mockFetch({
      movements: [line("payment", "2026-10-17", "Pago · Inscripción", 300000, 0, { method: "cash", depositId: "dep-1" })],
      totals: ZERO,
      dueTodayCents: 0,
      asOf: "2026-10-17",
    });

    await renderWithProviders(<AccountScreen />);

    fireEvent.press(await screen.findByRole("button", { name: "Movimiento Pago · Inscripción" }));
    expect(await screen.findByText("Este comprobante ya no está disponible.")).toBeTruthy();
    expect(sharePdf).not.toHaveBeenCalled();
  });

  it("lists every charge a payment paid, with its part", async () => {
    mockFetch({
      movements: [
        line("payment", "2026-10-17", "Pago · Inscripción 2026-2027 y 1 más", 350000, 0, {
          method: "cash",
          depositId: "dep-1",
          appliedTo: [
            { chargeId: "c1", description: "Inscripción 2026-2027", amountCents: 300000 },
            { chargeId: "c2", description: "Colegiatura octubre", amountCents: 50000 },
          ],
        }),
      ],
      totals: ZERO,
      dueTodayCents: 0,
      asOf: "2026-10-17",
    });

    await renderWithProviders(<AccountScreen />);

    expect(await screen.findByText("Pagó: Inscripción 2026-2027 $3,000.00 · Colegiatura octubre $500.00")).toBeTruthy();
  });

  it("does not offer a receipt for a line without a deposit", async () => {
    mockFetch({ movements: [line("charge", "2026-09-10", "Colegiatura septiembre", 320000, 320000)], totals: ZERO, dueTodayCents: 0, asOf: "2026-10-01" });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Colegiatura septiembre")).toBeTruthy());
    expect(screen.queryByRole("button", { name: "Movimiento Colegiatura septiembre" })).toBeNull();
  });

  // The figure is what makes the month current, early-payment reward included (ISSUE-T14-106).
  it("names today's figure as what keeps the family current", async () => {
    mockFetch({ movements: [], totals: { ...ZERO, balanceCents: 300000 }, dueTodayCents: 300000, asOf: "2026-10-17" });

    await renderWithProviders(<AccountScreen />);

    expect(await screen.findByText("Para estar al corriente")).toBeTruthy();
    expect(screen.queryByText("A pagar hoy")).toBeNull();
  });

  // Task 13: with the card's flag on, Cuenta leads to the child's card; off,
  // nothing hints that it exists.
  it("offers the card only with its flag on", async () => {
    mockFetch({ movements: [], totals: ZERO, dueTodayCents: 0, asOf: "2026-10-01" });
    await renderWithProviders(<AccountScreen />, { flags: { ...demoFlags, module_student_card: true } });

    fireEvent.press(await screen.findByLabelText("Credencial"));
    expect(router.push).toHaveBeenCalledWith("/credencial");
  });

  it("hides the card while its flag is off", async () => {
    mockFetch({ movements: [], totals: ZERO, dueTodayCents: 0, asOf: "2026-10-01" });
    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Al corriente")).toBeTruthy());
    expect(screen.queryByLabelText("Credencial")).toBeNull();
  });

  it("shows an empty state when there are no movements", async () => {
    mockFetch({ movements: [], totals: ZERO, dueTodayCents: 0, asOf: "2026-10-01" });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("Sin movimientos")).toBeTruthy());
  });

  it("shows an error state with retry when the statement load fails", async () => {
    mockFetch(null);

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar tu cuenta")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Reintentar" })).toBeTruthy();
  });

  // A statement with no day (should not happen from a T7 gateway) must not
  // become "$0.00 today" for a family that owes money (bug_005's rule).
  it("does not invent today's figure when the server did not send the day", async () => {
    mockFetch({ movements: [], totals: { ...ZERO, balanceCents: 3200000 } });

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByLabelText("Saldo").props.children).toBe("$32,000.00"));
    expect(screen.queryByText("Al corriente")).toBeNull();
  });

  it("shows the error state (not a fabricated settled balance) when the child bootstrap fails (bug_005)", async () => {
    const fetchMock = jest.fn(async (url: string) => {
      const u = String(url);
      if (u.includes("/family/students")) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<AccountScreen />);

    await waitFor(() => expect(screen.getByText("No pudimos cargar tu cuenta")).toBeTruthy());
    expect(screen.queryByText("Al corriente")).toBeNull();
    expect(screen.queryByText("$0.00")).toBeNull();
  });
});

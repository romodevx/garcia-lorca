import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { CardScreen } from "./CardScreen";
import { renderWithProviders } from "../test/renderWithProviders";

// The real QR renders SVG; here a stand-in says what it encodes. The real one
// is judged by the Metro bundle (bundling.test.ts), which Jest cannot be.
jest.mock("react-native-qrcode-svg", () => {
  const { Text } = jest.requireActual("react-native");
  return function QRCodeStandIn({ value }: { value: string }) {
    return <Text testID="card-qr">{value}</Text>;
  };
});

const child = { id: "s1", firstName: "Zoe", lastName: "Villalobos", level: "Primaria", gradeGroup: "1° A" };
const card = {
  studentId: "s1", name: "Zoe Villalobos", level: "Primaria", gradeGroup: "1° A", cycle: "2026-2027",
  enrollmentNumber: "2026-00042", school: { name: "COLEGIO GARCÍA LORCA", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" },
  qr: "GLCARD:AQAAAAAAAAAAAAAAAAAAAA",
};

function mockFetch(cardBody: unknown | null): void {
  globalThis.fetch = jest.fn(async (url: string) => {
    const u = String(url);
    if (u.includes("/family/students/s1/card")) {
      if (cardBody === null) return { ok: false, status: 500, json: async () => ({}) };
      return { ok: true, status: 200, json: async () => cardBody };
    }
    if (u.includes("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [child] }) };
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  }) as unknown as typeof fetch;
}

describe("CardScreen (tarea 13)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("shows the child's card, with the QR carrying only the opaque code", async () => {
    mockFetch(card);
    await renderWithProviders(<CardScreen />);

    expect(await screen.findByText("Zoe Villalobos")).toBeTruthy();
    expect(screen.getByText("2026-00042")).toBeTruthy();
    expect(screen.getByText(/1° A/)).toBeTruthy();
    expect(screen.getByText(/2026-2027/)).toBeTruthy();
    expect(screen.getByText("COLEGIO GARCÍA LORCA")).toBeTruthy();
    expect(screen.getByTestId("card-qr")).toHaveTextContent("GLCARD:AQAAAAAAAAAAAAAAAAAAAA");
  });

  it("shows the initials when the child has no photo", async () => {
    mockFetch(card);
    await renderWithProviders(<CardScreen />);

    const photo = await screen.findByTestId("card-photo");
    fireEvent(photo, "error");
    await waitFor(() => expect(screen.queryByTestId("card-photo")).toBeNull());
    expect(screen.getByText("ZV")).toBeTruthy();
  });

  it("says so when the card cannot be read, and offers to retry", async () => {
    mockFetch(null);
    await renderWithProviders(<CardScreen />);

    expect(await screen.findByText("Reintentar")).toBeTruthy();
  });

  // Owner's decision: a revoked card stays suspended until the office issues a
  // new one; the app shows no code meanwhile.
  it("says the card is suspended, and shows no QR", async () => {
    mockFetch({ ...card, qr: "", suspended: true });
    await renderWithProviders(<CardScreen />);

    expect(await screen.findByText(/Credencial suspendida/)).toBeTruthy();
    expect(screen.queryByTestId("card-qr")).toBeNull();
  });

  it("shows the school's logo and contact data, never its RFC or legal name", async () => {
    mockFetch({
      ...card,
      school: {
        name: "COLEGIO GARCÍA LORCA", legalName: "CENTRO EDUCATIVO GL SC", rfc: "CEG010101AB1",
        cct: "16PPR0123X", address: "Av. Madero 100, Morelia", phone: "443 123 4567", legend: "Válida durante el ciclo escolar",
      },
    });
    await renderWithProviders(<CardScreen />);

    expect(await screen.findByTestId("card-school-logo")).toBeTruthy();
    expect(screen.getByText(/16PPR0123X/)).toBeTruthy();
    expect(screen.getByText("Av. Madero 100, Morelia")).toBeTruthy();
    expect(screen.getByText(/443 123 4567/)).toBeTruthy();
    expect(screen.getByText("Válida durante el ciclo escolar")).toBeTruthy();
    expect(screen.queryByText(/CEG010101AB1/)).toBeNull();
    expect(screen.queryByText(/CENTRO EDUCATIVO GL SC/)).toBeNull();
    expect(screen.queryByLabelText(/CENTRO EDUCATIVO GL SC/)).toBeNull();
  });

  it("leaves out the school data the school has not filled in", async () => {
    mockFetch(card);
    await renderWithProviders(<CardScreen />);

    expect(await screen.findByText("Zoe Villalobos")).toBeTruthy();
    expect(screen.queryByText(/CCT/)).toBeNull();
    expect(screen.queryByText(/Tel\./)).toBeNull();
    expect(screen.queryByTestId("card-school-footer")).toBeNull();
  });
});

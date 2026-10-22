import { act, fireEvent, screen, waitFor } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "../api";
import { pickVoucherFile, type VoucherPick } from "../files/pickVoucher";
import { renderWithProviders } from "../test/renderWithProviders";
import { VoucherScreen } from "./VoucherScreen";

jest.mock("../files/pickVoucher", () => ({ pickVoucherFile: jest.fn() }));

const pick = jest.mocked(pickVoucherFile);
const child = { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "primaria", gradeGroup: "A" };
const TODAY = "2026-10-21";
const PHOTO: VoucherPick = { kind: "picked", file: { uri: "file:///cache/IMG_0004.jpg", name: "IMG_0004.jpg", type: "image/jpeg" } };
const PENDING = { id: "v1", status: "pending", claimedAmountCents: 250000, claimedPaidOn: TODAY, createdAt: "2026-10-21T18:00:00Z" };

interface Reply {
  status: number;
  body?: unknown;
  headers?: Record<string, string>;
}

function reply({ status, body = {}, headers = {} }: Reply): object {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (n: string) => headers[n.toLowerCase()] ?? null },
    json: async () => body,
  };
}

/** Answers the child bootstrap, the voucher list (`listed` in turn) and the upload (`sent`). */
function mockGateway(sent: Reply, listed: unknown[][] = [[], [PENDING]], listStatus = 200): { posts: () => RequestInit[] } {
  const posts: RequestInit[] = [];
  let reads = 0;
  globalThis.fetch = jest.fn(async (url: string, init?: RequestInit) => {
    const u = String(url);
    if (u.endsWith("/family/students/s1/payment-vouchers")) {
      if (init?.method === "POST") {
        posts.push(init);
        return reply(sent);
      }
      const vouchers = listed[Math.min(reads++, listed.length - 1)];
      return reply({ status: listStatus, body: { vouchers } });
    }
    if (u.endsWith("/family/students")) return reply({ status: 200, body: { students: [child] } });
    return reply({ status: 200, body: { notifications: [] } });
  }) as unknown as typeof fetch;
  return { posts: () => posts };
}

async function renderScreen(): Promise<void> {
  await renderWithProviders(<VoucherScreen today={TODAY} />);
  await waitFor(() => expect(screen.getByText("Aún no has enviado comprobantes")).toBeTruthy());
}

async function choosePhoto(): Promise<void> {
  pick.mockResolvedValue(PHOTO);
  await fireEvent.press(screen.getByLabelText("Elegir de la galería"));
}

async function send(): Promise<void> {
  await fireEvent.press(screen.getByLabelText("Enviar comprobante"));
}

describe("VoucherScreen (¿Ya pagaste? Sube tu comprobante)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
    jest.clearAllMocks();
  });

  it("sends the picked photo with today as the payment day, then lists it as pending", async () => {
    const gateway = mockGateway({ status: 201, body: { voucher: PENDING, replayed: false } });
    await renderScreen();

    expect(screen.getByLabelText("Fecha de pago").props.value).toBe("21/10/2026");
    await choosePhoto();
    expect(screen.getByText("IMG_0004.jpg")).toBeTruthy();
    await fireEvent.changeText(screen.getByLabelText("Importe pagado"), "2,500");
    await send();

    await waitFor(() => expect(screen.getByText(/Listo, enviamos tu comprobante/)).toBeTruthy());
    expect(gateway.posts()).toHaveLength(1);
    expect(await screen.findByLabelText("Comprobante pendiente")).toBeTruthy();
    // The form is ready for the next one.
    expect(screen.queryByText("IMG_0004.jpg")).toBeNull();
  });

  it("asks for the file on submit and sends nothing without it", async () => {
    const gateway = mockGateway({ status: 201, body: { voucher: PENDING, replayed: false } });
    await renderScreen();

    await send();

    expect(screen.getAllByText("Elige la foto o el PDF del comprobante").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Revisa el formulario")).toBeTruthy();
    expect(gateway.posts()).toHaveLength(0);
  });

  it("flags an unreadable amount when leaving the field and clears it once fixed", async () => {
    mockGateway({ status: 201 });
    await renderScreen();
    const amount = screen.getByLabelText("Importe pagado");

    await fireEvent.changeText(amount, "abc");
    expect(screen.queryByText(/Escribe el importe/)).toBeNull();
    await fireEvent(amount, "blur");
    expect(screen.getByText("Escribe el importe en pesos, por ejemplo 2,500.00")).toBeTruthy();
    await fireEvent.changeText(amount, "2500");
    expect(screen.queryByText(/Escribe el importe/)).toBeNull();
  });

  it("refuses a payment day after today", async () => {
    mockGateway({ status: 201 });
    await renderScreen();
    const day = screen.getByLabelText("Fecha de pago");

    await fireEvent.changeText(day, "22/10/2026");
    await fireEvent(day, "blur");

    expect(screen.getByText("La fecha no puede ser después de hoy")).toBeTruthy();
  });

  it.each([
    [{ status: 413 }, "El archivo pesa más de 5 MB. Elige una foto más ligera o un PDF más pequeño."],
    [{ status: 409, headers: { "x-error-code": "VOUCHER_LIMIT" } }, "Ya tienes 3 comprobantes por revisar para este alumno. Espera a que la escuela los revise."],
    [{ status: 429, headers: { "retry-after": "60" } }, "Enviaste varios comprobantes seguidos. Espera 1 minuto e inténtalo de nuevo."],
  ] as [Reply, string][])("words a refused upload (%j) for the family and keeps the file", async (refusal, message) => {
    mockGateway(refusal);
    await renderScreen();

    await choosePhoto();
    await send();

    await waitFor(() => expect(screen.getByText(message)).toBeTruthy());
    expect(screen.getByText("IMG_0004.jpg")).toBeTruthy();
  });

  it("sends once however many times the button is tapped, and never disables it", async () => {
    let finish: (value: object) => void = () => undefined;
    const gateway = mockGateway({ status: 201 });
    const base = globalThis.fetch;
    globalThis.fetch = jest.fn((url: string, init?: RequestInit) =>
      init?.method === "POST" ? new Promise((resolve) => { gateway.posts().push(init); finish = resolve; }) : base(url, init),
    ) as unknown as typeof fetch;
    await renderScreen();
    await choosePhoto();

    await send();
    await send();

    expect(gateway.posts()).toHaveLength(1);
    const button = screen.getByLabelText("Enviar comprobante");
    expect(button.props.accessibilityState?.disabled).toBe(false);
    expect(screen.getByText("Enviando…")).toBeTruthy();
    await act(async () => finish(reply({ status: 201, body: { voucher: PENDING, replayed: false } })));
  });

  it("tells the family when the same file was already sent", async () => {
    mockGateway({ status: 200, body: { voucher: PENDING, replayed: true } });
    await renderScreen();

    await choosePhoto();
    await send();

    await waitFor(() => expect(screen.getByText("Ya habías enviado este comprobante; no lo mandamos de nuevo.")).toBeTruthy());
  });

  it("explains a refused camera instead of doing nothing", async () => {
    mockGateway({ status: 201 });
    await renderScreen();
    pick.mockResolvedValue({ kind: "denied" });

    await fireEvent.press(screen.getByLabelText("Tomar foto"));

    expect(screen.getByText("Permite el acceso a la cámara en Ajustes para tomar la foto.")).toBeTruthy();
  });

  it("refuses a format the school does not take, in words", async () => {
    const gateway = mockGateway({ status: 201 });
    await renderScreen();
    pick.mockResolvedValue({ kind: "unsupported" });

    await fireEvent.press(screen.getByLabelText("Elegir de la galería"));
    await send();

    expect(screen.getAllByText("Ese formato no se puede enviar. Usa una foto JPG o PNG, o un PDF.").length).toBeGreaterThan(0);
    expect(gateway.posts()).toHaveLength(0);
  });

  it("drops the earlier file when a later pick is refused, so it is never sent by mistake", async () => {
    const gateway = mockGateway({ status: 201, body: { voucher: PENDING, replayed: false } });
    await renderScreen();
    await choosePhoto();
    expect(screen.getByLabelText("Archivo a enviar: IMG_0004.jpg")).toBeTruthy();

    pick.mockResolvedValue({ kind: "unsupported" });
    await fireEvent.press(screen.getByLabelText("Elegir de la galería"));
    await send();

    expect(screen.queryByLabelText(/^Archivo a enviar/)).toBeNull();
    expect(gateway.posts()).toHaveLength(0);
  });

  it("says the feature is off when the gateway answers 404", async () => {
    mockGateway({ status: 404 }, [[]], 404);
    await renderWithProviders(<VoucherScreen today={TODAY} />);

    expect(await screen.findByText("El envío de comprobantes no está disponible por ahora.")).toBeTruthy();
    expect(screen.queryByLabelText("Enviar comprobante")).toBeNull();
  });
});

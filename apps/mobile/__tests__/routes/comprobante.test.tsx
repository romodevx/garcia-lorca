import { screen } from "@testing-library/react-native";
import { clearTokens, saveTokens } from "@/src/api";
import { demoFlags } from "@/src/flags/bootstrap";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import ComprobanteRoute from "../../app/comprobante";

jest.mock("@/src/files/pickVoucher", () => ({ pickVoucherFile: jest.fn() }));

describe("ComprobanteRoute (ADR 0040)", () => {
  beforeEach(async () => {
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("is on in this build's seed and shows the voucher screen under its title", async () => {
    expect(demoFlags.module_payment_vouchers).toBe(true);
    await renderWithProviders(<ComprobanteRoute />);

    expect(screen.getByText("Subir comprobante")).toBeTruthy();
    expect(await screen.findByLabelText("Enviar comprobante")).toBeTruthy();
  });

  it("shows nothing of the form with the module's flag off", async () => {
    await renderWithProviders(<ComprobanteRoute />, { flags: { ...demoFlags, module_payment_vouchers: false } });

    expect(screen.getByText("Módulo no disponible")).toBeTruthy();
    expect(screen.queryByLabelText("Enviar comprobante")).toBeNull();
  });
});

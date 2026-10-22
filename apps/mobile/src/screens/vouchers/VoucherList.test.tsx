import { screen, within } from "@testing-library/react-native";
import type { FamilyPaymentVoucher } from "../../api";
import { renderWithProviders } from "../../test/renderWithProviders";
import { VoucherList } from "./VoucherList";

const sent = (extra: Partial<FamilyPaymentVoucher>): FamilyPaymentVoucher => ({
  id: "v",
  status: "pending",
  createdAt: "2026-10-21T15:00:00Z",
  ...extra,
});

describe("VoucherList (Mis comprobantes)", () => {
  it("shows each voucher's status in the API's order, newest first", async () => {
    await renderWithProviders(
      <VoucherList
        vouchers={[
          sent({ id: "v3", status: "pending", claimedAmountCents: 250000, claimedPaidOn: "2026-10-20" }),
          sent({ id: "v2", status: "rejected", rejectionReason: "La foto no se lee", createdAt: "2026-10-10T15:00:00Z" }),
          sent({ id: "v1", status: "applied", depositFolio: "P-2026-000042", createdAt: "2026-10-01T15:00:00Z" }),
        ]}
      />,
    );

    const rows = screen.getAllByLabelText(/^Comprobante /);
    const [newest, middle, oldest] = rows as [(typeof rows)[0], (typeof rows)[0], (typeof rows)[0]];
    expect(rows.map((r) => r.props.accessibilityLabel)).toEqual([
      "Comprobante pendiente",
      "Comprobante rechazado",
      "Comprobante aplicado",
    ]);
    expect(within(newest).getByText("Pendiente")).toBeTruthy();
    expect(within(newest).getByText("$2,500.00 · pagado el 20 oct 2026")).toBeTruthy();
    expect(within(middle).getByText("Rechazado")).toBeTruthy();
    expect(within(middle).getByText("Motivo: La foto no se lee")).toBeTruthy();
    expect(within(oldest).getByText("Aplicado")).toBeTruthy();
    expect(within(oldest).getByText("Recibo P-2026-000042")).toBeTruthy();
  });

  it("dates a voucher by the school's day it was sent", async () => {
    // 03:00 UTC on the 21st is still the 20th in Morelia.
    await renderWithProviders(<VoucherList vouchers={[sent({ createdAt: "2026-10-21T03:00:00Z" })]} />);

    expect(screen.getByText("Enviado el 20 oct 2026")).toBeTruthy();
  });

  it("says so when nothing was sent yet", async () => {
    await renderWithProviders(<VoucherList vouchers={[]} />);

    expect(screen.getByText("Aún no has enviado comprobantes")).toBeTruthy();
  });
});

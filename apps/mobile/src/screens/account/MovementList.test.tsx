import { fireEvent, screen } from "@testing-library/react-native";

import type { FamilyMovement } from "../../api";
import { renderWithProviders } from "../../test/renderWithProviders";
import { MovementList } from "./MovementList";

const payment = {
  kind: "payment",
  date: "2026-10-17",
  description: "Pago · Inscripción",
  amountCents: 300000,
  method: "card",
} as FamilyMovement;

// ISSUE-T14-402: the line already says "Pago · …", so its second line must
// not say "Pago" again.
describe("MovementList", () => {
  it("does not repeat the kind when the description already names it", async () => {
    await renderWithProviders(<MovementList movements={[payment]} />);
    expect(screen.getByText("Pago · Inscripción")).toBeTruthy();
    expect(screen.getByText("17 oct · Tarjeta")).toBeTruthy();
  });

  it("keeps the kind when the description does not name it", async () => {
    await renderWithProviders(<MovementList movements={[{ ...payment, description: "Colegiatura octubre" }]} />);
    expect(screen.getByText("Pago · 17 oct · Tarjeta")).toBeTruthy();
  });

  // ISSUE-CC-315: one deposit paid cash + card is ONE movement with its tenders,
  // and what it paid is said once — not once per tender ($7,500 read twice).
  describe("a deposit paid in two tenders", () => {
    const applied = [
      { chargeId: "c1", description: "INSCRIPCIÓN", amountCents: 400000 },
      { chargeId: "c2", description: "Colegiatura octubre", amountCents: 350000 },
    ];
    const tender = (method: string, amountCents: number): FamilyMovement =>
      ({
        kind: "payment",
        date: "2026-10-17",
        description: "Pago · INSCRIPCIÓN y 1 más",
        amountCents,
        method,
        folio: "P-2026-000001",
        depositId: "d1",
        appliedTo: applied,
      }) as FamilyMovement;
    const lines = [tender("cash", 500000), tender("card", 250000)];

    it("renders one row with the deposit's total and each tender's part", async () => {
      await renderWithProviders(<MovementList movements={lines} onOpenReceipt={jest.fn()} />);
      expect(screen.getAllByLabelText("Movimiento Pago · INSCRIPCIÓN y 1 más")).toHaveLength(1);
      expect(screen.getByText("-$7,500.00")).toBeTruthy();
      expect(screen.getByText("17 oct · Efectivo $5,000.00 · Tarjeta $2,500.00 · P-2026-000001")).toBeTruthy();
    });

    it("says what the deposit paid only once", async () => {
      await renderWithProviders(<MovementList movements={lines} />);
      expect(screen.getAllByText("Pagó: INSCRIPCIÓN $4,000.00 · Colegiatura octubre $3,500.00")).toHaveLength(1);
    });

    it("opens the deposit's receipt from its single row", async () => {
      const onOpenReceipt = jest.fn();
      await renderWithProviders(<MovementList movements={lines} onOpenReceipt={onOpenReceipt} />);
      fireEvent.press(screen.getByLabelText("Movimiento Pago · INSCRIPCIÓN y 1 más"));
      expect(onOpenReceipt).toHaveBeenCalledWith(expect.objectContaining({ depositId: "d1" }));
    });
  });
});

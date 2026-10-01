import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Receipt } from "../../receipts/api";
import { ReceiptTicket } from "./ReceiptTicket";

const receipt: Receipt = {
  kind: "payment", folio: "P-2026-000042",
  school: { name: "COLEGIO GARCÍA LORCA", legalName: "EGL S.C.", rfc: "EGL150101AB1", cct: "", address: "CENTRO", phone: "5512345678", legend: "Gracias por su pago." },
  paidOn: "2026-10-10", buyer: "Camila Zamora Sánchez", buyerDetail: "1° A",
  lines: [
    { description: "Colegiatura septiembre 2026", amountCents: 310000, discountCents: 10000 },
    { description: "Recargo · Colegiatura septiembre 2026", amountCents: 30000, surcharge: true },
  ],
  surchargeCents: 30000, discountCents: 10000, creditCents: 10000, totalCents: 350000,
  method: "Tarjeta de débito", reference: "1234", receivedBy: "Laura Caja",
};

describe("ReceiptTicket (tarea 12)", () => {
  it("says what the receipt says, for an 80 mm roll", () => {
    render(<ReceiptTicket receipt={receipt} />);

    expect(screen.getByText("COLEGIO GARCÍA LORCA")).toBeInTheDocument();
    expect(screen.getByText(/RFC EGL150101AB1/)).toBeInTheDocument();
    expect(screen.getByText("P-2026-000042")).toBeInTheDocument();
    expect(screen.getByText(/Camila Zamora Sánchez/)).toBeInTheDocument();
    expect(screen.getByText("Colegiatura septiembre 2026")).toBeInTheDocument();
    expect(screen.getByTestId("ticket-total")).toHaveTextContent("$3,500.00");
    expect(screen.getByText(/Saldo a favor/)).toBeInTheDocument();
    expect(screen.getByText(/Tarjeta de débito/)).toBeInTheDocument();
    expect(screen.getByText(/Laura Caja/)).toBeInTheDocument();
    expect(screen.getByText("Gracias por su pago.")).toBeInTheDocument();
    expect(screen.queryByText("CANCELADO")).toBeNull();
  });

  it("says CANCELADO on a voided one", () => {
    render(<ReceiptTicket receipt={{ ...receipt, voided: true, voidReason: "capturado mal" }} />);

    expect(screen.getByText("CANCELADO")).toBeInTheDocument();
    expect(screen.getByText(/capturado mal/)).toBeInTheDocument();
  });

  // T9: both receipts of a correction print again, each naming the other.
  it("says SUSTITUIDO on a corrected one, and names its replacement", () => {
    render(<ReceiptTicket receipt={{ ...receipt, superseded: true, supersededByFolio: "P-2026-000002" }} />);

    expect(screen.getByText("SUSTITUIDO")).toBeInTheDocument();
    expect(screen.getByText(/Sustituido por P-2026-000002/)).toBeInTheDocument();
  });

  it("names the receipt a correction replaces", () => {
    render(<ReceiptTicket receipt={{ ...receipt, correctsFolio: "P-2026-000001" }} />);

    expect(screen.getByText(/Corrige a P-2026-000001/)).toBeInTheDocument();
    expect(screen.queryByText("SUSTITUIDO")).toBeNull();
  });

  it("titles a uniforms sale as a sale", () => {
    render(<ReceiptTicket receipt={{ ...receipt, kind: "sale", folio: "U-2026-000007" }} />);

    expect(screen.getByText("Comprobante de venta")).toBeInTheDocument();
  });
});

describe("ReceiptTicket · formas de pago (tarea 8)", () => {
  it("prints one row per tender when the payment was split", () => {
    render(
      <ReceiptTicket
        receipt={{
          ...receipt,
          method: "Nota de crédito + Efectivo",
          reference: undefined,
          tenders: [
            { method: "Nota de crédito", amountCents: 50000, reference: "NC-12" },
            { method: "Efectivo", amountCents: 300000 },
          ],
        }}
      />,
    );

    const rows = screen.getAllByTestId("ticket-tender");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent(/Nota de crédito.*ref\. NC-12.*\$500\.00/);
    expect(rows[1]).toHaveTextContent(/Efectivo.*\$3,000\.00/);
  });

  it("prints a lone credit note as its own row too, known by its code and not its wording", () => {
    render(<ReceiptTicket receipt={{ ...receipt, method: "NC", tenders: [{ code: "credit_note", method: "NC", amountCents: 350000 }] }} />);
    expect(screen.getAllByTestId("ticket-tender")).toHaveLength(1);
  });

  it("keeps the single line for one ordinary method", () => {
    render(<ReceiptTicket receipt={{ ...receipt, tenders: [{ method: "Tarjeta de débito", amountCents: 350000, reference: "1234" }] }} />);
    expect(screen.queryByTestId("ticket-tender")).toBeNull();
    expect(screen.getByText(/Forma de pago: Tarjeta de débito · ref\. 1234/)).toBeInTheDocument();
  });
});

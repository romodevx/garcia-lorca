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

  it("titles a uniforms sale as a sale", () => {
    render(<ReceiptTicket receipt={{ ...receipt, kind: "sale", folio: "U-2026-000007" }} />);

    expect(screen.getByText("Comprobante de venta")).toBeInTheDocument();
  });
});

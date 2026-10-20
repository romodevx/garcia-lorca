import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as uniformsApi from "../../uniforms/api";
import { ReturnSalePanel, soldOnSchoolDay } from "./ReturnSalePanel";

vi.mock("../../uniforms/api", async (importOriginal) => {
  const actual = await importOriginal<typeof uniformsApi>();
  return { ...actual, returnCounterSale: vi.fn() };
});
const uniforms = vi.mocked(uniformsApi);

const ORDER: uniformsApi.UniformOrder = {
  id: "ord-1", folio: "U-2026-000002", studentName: "PRUEBA CALIDAD ALUMNA DOS", source: "counter", label: "closed",
  totalCents: 25000, createdAt: "2026-10-20T00:21:42Z", // 18:21 of the 19th in Morelia
  lines: [{ id: "ln-1", itemId: "it-1", itemName: "PLAYERA DEPORTIVA QA", sizeLabel: "8", qty: 1, unitPriceCents: 25000, status: "sold", fulfillmentId: "ful-1" }],
  fulfillments: [{ id: "ful-1", seq: 1, folio: "U-2026-000002", status: "sold_at_counter", amountCents: 25000, deadlineEstimated: false, requiresAttention: false }],
};
const SAME_DAY = new Date("2026-10-20T01:00:00Z"); // 19:00 of the 19th in Morelia
const NEXT_DAY = new Date("2026-10-20T15:00:00Z"); // 09:00 of the 20th

afterEach(() => vi.clearAllMocks());

describe("soldOnSchoolDay", () => {
  // At 19:00 in Morelia the UTC date is already tomorrow: the school's day decides.
  it("judges the day in Morelia, not in UTC", () => {
    expect(soldOnSchoolDay("2026-10-20T00:21:42Z", SAME_DAY)).toBe(true);
    expect(soldOnSchoolDay("2026-10-20T00:21:42Z", NEXT_DAY)).toBe(false);
  });
});

describe("ReturnSalePanel (C2-015)", () => {
  it("the day of the sale cancels the charge, and says so", async () => {
    const user = userEvent.setup();
    const onDone = vi.fn();
    uniforms.returnCounterSale.mockResolvedValue({ ...ORDER, label: "returned" });
    render(<ReturnSalePanel order={ORDER} now={() => SAME_DAY} onDone={onDone} onClose={vi.fn()} />);

    expect(screen.getByText(/se cancela el cobro/i)).toBeInTheDocument();
    expect(screen.queryByRole("radiogroup", { name: /cómo se regresa el dinero/i })).toBeNull();
    await user.type(screen.getByLabelText(/^Motivo/), "Talla equivocada");
    await user.click(screen.getByRole("checkbox", { name: /PLAYERA DEPORTIVA QA · talla 8 × 1 viene dañada/ }));
    await user.click(screen.getByRole("button", { name: "Devolver la venta" }));

    await waitFor(() =>
      expect(uniforms.returnCounterSale).toHaveBeenCalledWith(expect.any(String), "ord-1", { reason: "Talla equivocada", damagedLineIds: ["ln-1"] }),
    );
    expect(onDone).toHaveBeenCalledWith(expect.objectContaining({ label: "returned" }), "Venta U-2026-000002 devuelta.");
  });

  it("another day asks how the money goes back, cash by default", async () => {
    const user = userEvent.setup();
    uniforms.returnCounterSale.mockResolvedValue(ORDER);
    render(<ReturnSalePanel order={ORDER} now={() => NEXT_DAY} onDone={vi.fn()} onClose={vi.fn()} />);

    expect(screen.getByText(/egreso de hoy/i)).toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "Transferencia" }));
    await user.type(screen.getByLabelText(/^Motivo/), "Venía descosida");
    await user.click(screen.getByRole("button", { name: "Devolver la venta" }));

    await waitFor(() =>
      expect(uniforms.returnCounterSale).toHaveBeenCalledWith(expect.any(String), "ord-1", {
        reason: "Venía descosida", damagedLineIds: [], refundMethod: "transfer",
      }),
    );
  });

  it("asks for the reason on submit, without disabling the button", async () => {
    const user = userEvent.setup();
    render(<ReturnSalePanel order={ORDER} now={() => SAME_DAY} onDone={vi.fn()} onClose={vi.fn()} />);

    const submit = screen.getByRole("button", { name: "Devolver la venta" });
    expect(submit).toBeEnabled();
    await user.click(submit);

    expect(await screen.findByText(/escribe el motivo/i)).toBeInTheDocument();
    expect(uniforms.returnCounterSale).not.toHaveBeenCalled();
  });

  it("shows the server's refusal in Spanish", async () => {
    const user = userEvent.setup();
    uniforms.returnCounterSale.mockRejectedValue(new uniformsApi.UniformsError("conflict", "Esa venta ya se devolvió."));
    render(<ReturnSalePanel order={ORDER} now={() => SAME_DAY} onDone={vi.fn()} onClose={vi.fn()} />);

    await user.type(screen.getByLabelText(/^Motivo/), "Talla equivocada");
    await user.click(screen.getByRole("button", { name: "Devolver la venta" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Esa venta ya se devolvió.");
  });
});

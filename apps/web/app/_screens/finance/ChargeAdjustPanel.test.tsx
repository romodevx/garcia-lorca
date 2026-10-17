import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { ChargeAdjustPanel } from "./ChargeAdjustPanel";

vi.mock("../../finance/api", () => ({ applyDiscount: vi.fn(), cancelCharge: vi.fn() }));

const finance = vi.mocked(financeApi);

const CHARGE: financeApi.Charge = {
  id: "ch1", studentId: "s1", cycleId: "c1", conceptId: "k1", description: "Colegiatura octubre",
  amountCents: 350000, discountCents: 0, dueDate: "2026-10-10", status: "pending", paidCents: 0, remainingCents: 350000, scholarshipCents: 0,
};

function setup(): { onDone: ReturnType<typeof vi.fn>; onClose: ReturnType<typeof vi.fn> } {
  const onDone = vi.fn();
  const onClose = vi.fn();
  render(<ChargeAdjustPanel charge={CHARGE} name="CRUZ VALENTINA" onDone={onDone} onClose={onClose} />);
  return { onDone, onClose };
}

afterEach(() => vi.clearAllMocks());

describe("ChargeAdjustPanel · descuento (CC-203)", () => {
  it("applies a discount in pesos with its reason", async () => {
    const user = userEvent.setup();
    const discounted = { ...CHARGE, discountCents: 50000, remainingCents: 300000 };
    finance.applyDiscount.mockResolvedValue(discounted);
    const { onDone } = setup();

    await user.type(screen.getByLabelText(/^Monto del descuento/), "500");
    await user.type(screen.getByLabelText(/^Motivo/), "Hermano en el colegio");
    await user.click(screen.getByRole("button", { name: "Aplicar descuento" }));

    await waitFor(() => expect(finance.applyDiscount).toHaveBeenCalledWith(expect.any(String), "ch1", 50000, "Hermano en el colegio"));
    expect(onDone).toHaveBeenCalledWith(discounted, "Descuento aplicado a CRUZ VALENTINA.");
  });

  it("asks for what is missing on submit, never disabling the button", async () => {
    const user = userEvent.setup();
    setup();

    const apply = screen.getByRole("button", { name: "Aplicar descuento" });
    expect(apply).toBeEnabled();
    await user.click(apply);

    expect(await screen.findAllByText("Falta este dato")).toHaveLength(2);
    expect(finance.applyDiscount).not.toHaveBeenCalled();
  });

  it("refuses a discount larger than the charge, naming the amount", async () => {
    const user = userEvent.setup();
    setup();

    await user.type(screen.getByLabelText(/^Monto del descuento/), "4000");
    await user.type(screen.getByLabelText(/^Motivo/), "Error");
    await user.click(screen.getByRole("button", { name: "Aplicar descuento" }));

    expect(await screen.findByText(/no puede ser mayor que el importe \(\$3,500\.00\)/i)).toBeInTheDocument();
    expect(finance.applyDiscount).not.toHaveBeenCalled();
  });

  it("keeps the form and says so when the server refuses", async () => {
    const user = userEvent.setup();
    finance.applyDiscount.mockRejectedValue(new Error("discount and scholarship cannot exceed amount"));
    const { onDone } = setup();

    await user.type(screen.getByLabelText(/^Monto del descuento/), "500");
    await user.type(screen.getByLabelText(/^Motivo/), "Hermano");
    await user.click(screen.getByRole("button", { name: "Aplicar descuento" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudo aplicar el descuento/);
    expect(screen.getByLabelText(/^Motivo/)).toHaveValue("Hermano");
    expect(onDone).not.toHaveBeenCalled();
  });
});

describe("ChargeAdjustPanel · cancelar (CC-203)", () => {
  it("warns that it does not undo and cancels with the reason", async () => {
    const user = userEvent.setup();
    const cancelled = { ...CHARGE, status: "cancelled" as const, remainingCents: 0 };
    finance.cancelCharge.mockResolvedValue(cancelled);
    const { onDone } = setup();

    await user.click(screen.getByRole("radio", { name: "Cancelar cargo" }));
    expect(screen.getByText(/no se puede deshacer/i)).toBeInTheDocument();
    await user.type(screen.getByLabelText(/^Motivo/), "Cargo duplicado");
    await user.click(screen.getByRole("button", { name: "Cancelar este cargo" }));

    await waitFor(() => expect(finance.cancelCharge).toHaveBeenCalledWith(expect.any(String), "ch1", "Cargo duplicado"));
    expect(onDone).toHaveBeenCalledWith(cancelled, "Cargo de CRUZ VALENTINA cancelado.");
  });

  it("will not cancel without a reason", async () => {
    const user = userEvent.setup();
    setup();

    await user.click(screen.getByRole("radio", { name: "Cancelar cargo" }));
    await user.click(screen.getByRole("button", { name: "Cancelar este cargo" }));

    expect(await screen.findByText("Falta este dato")).toBeInTheDocument();
    expect(finance.cancelCharge).not.toHaveBeenCalled();
  });

  it.each([
    [Object.assign(new Error("charge already cancelled"), { status: 409 }), /ya estaba cancelado/],
    [Object.assign(new Error("forbidden"), { status: 403 }), /no tiene permiso/],
    [new TypeError("Failed to fetch"), /No hubo conexión/],
  ])("tells a conflict, a refusal and a lost connection apart (%#)", async (cause, message) => {
    const user = userEvent.setup();
    finance.cancelCharge.mockRejectedValue(cause);
    setup();

    await user.click(screen.getByRole("radio", { name: "Cancelar cargo" }));
    await user.type(screen.getByLabelText(/^Motivo/), "Cargo duplicado");
    await user.click(screen.getByRole("button", { name: "Cancelar este cargo" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(message);
  });

  it("closing leaves the charge untouched", async () => {
    const user = userEvent.setup();
    const { onClose } = setup();

    await user.click(screen.getByRole("button", { name: "Cerrar ajustes" }));

    expect(onClose).toHaveBeenCalled();
  });
});

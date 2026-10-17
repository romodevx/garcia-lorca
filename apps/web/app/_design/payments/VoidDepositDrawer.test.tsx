import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { VoidDepositDrawer } from "./VoidDepositDrawer";

vi.mock("../../finance/api", () => ({ voidDeposit: vi.fn() }));

const finance = vi.mocked(financeApi);

afterEach(() => vi.clearAllMocks());

function setup(): { onVoided: ReturnType<typeof vi.fn>; onCancel: ReturnType<typeof vi.fn> } {
  const onVoided = vi.fn();
  const onCancel = vi.fn();
  render(<VoidDepositDrawer depositId="d1" folio="P-2026-000005" onVoided={onVoided} onCancel={onCancel} />);
  return { onVoided, onCancel };
}

describe("VoidDepositDrawer (CC-203)", () => {
  it("says it cannot be undone and voids with the reason", async () => {
    const user = userEvent.setup();
    finance.voidDeposit.mockResolvedValue();
    const { onVoided } = setup();

    const dialog = await screen.findByRole("dialog", { name: /Anular pago P-2026-000005/ });
    expect(dialog).toHaveTextContent(/no se puede deshacer/i);
    await user.type(screen.getByLabelText(/^Motivo/), "Se capturó $99,999 por error");
    await user.click(screen.getByRole("button", { name: "Anular pago" }));

    await waitFor(() => expect(finance.voidDeposit).toHaveBeenCalledWith(expect.any(String), "d1", "Se capturó $99,999 por error"));
    expect(onVoided).toHaveBeenCalledWith("Pago P-2026-000005 anulado. Lo que pagó vuelve a deberse.");
  });

  it("will not void without a reason, and the button stays enabled", async () => {
    const user = userEvent.setup();
    setup();

    const button = await screen.findByRole("button", { name: "Anular pago" });
    expect(button).toBeEnabled();
    await user.click(button);

    expect(await screen.findByText("Falta este dato")).toBeInTheDocument();
    expect(finance.voidDeposit).not.toHaveBeenCalled();
  });

  it("keeps the reason and says so when the server refuses", async () => {
    const user = userEvent.setup();
    finance.voidDeposit.mockRejectedValue(new Error("this deposit is already cancelled"));
    const { onVoided } = setup();

    await user.type(await screen.findByLabelText(/^Motivo/), "Duplicado");
    await user.click(screen.getByRole("button", { name: "Anular pago" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudo anular el pago/);
    expect(screen.getByLabelText(/^Motivo/)).toHaveValue("Duplicado");
    expect(onVoided).not.toHaveBeenCalled();
  });

  it.each([
    [Object.assign(new Error("this deposit is already cancelled"), { status: 409 }), /ya estaba anulado o corregido/],
    [Object.assign(new Error("forbidden"), { status: 403 }), /no tiene permiso para anular pagos/],
    [new TypeError("Failed to fetch"), /No hubo conexión/],
  ])("tells a conflict, a refusal and a lost connection apart (%#)", async (cause, message) => {
    const user = userEvent.setup();
    finance.voidDeposit.mockRejectedValue(cause);
    setup();

    await user.type(await screen.findByLabelText(/^Motivo/), "Duplicado");
    await user.click(screen.getByRole("button", { name: "Anular pago" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(message);
  });

  it("backing out leaves the payment alone", async () => {
    const user = userEvent.setup();
    const { onCancel } = setup();

    await user.click(await screen.findByRole("button", { name: "No anular" }));

    expect(onCancel).toHaveBeenCalled();
    expect(finance.voidDeposit).not.toHaveBeenCalled();
  });
});

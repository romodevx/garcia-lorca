import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { TenderEditor } from "./TenderEditor";
import { useTenders } from "./useTenders";
import type { Tender } from "../../finance/api";

/** The editor as a screen holds it: the hook owns the rows, the screen asks it to validate. */
function Harness({ totalCents, onValid }: { totalCents: number; onValid?: (t: Tender[]) => void }): ReactElement {
  const tenders = useTenders(totalCents);
  return (
    <>
      <TenderEditor tenders={tenders} />
      <button type="button" onClick={() => { if (tenders.validate()) onValid?.(tenders.toTenders()); }}>Guardar</button>
    </>
  );
}

const row = (n: number): HTMLElement => screen.getByRole("group", { name: `Forma de pago ${n}` });
const amountOf = (n: number): HTMLElement => within(row(n)).getByLabelText("Monto");

describe("TenderEditor (tarea 8)", () => {
  // The 80% case is still one click: cash, for the whole amount.
  it("starts with one cash row for the whole amount, and follows the amount until it is edited", () => {
    const { rerender } = render(<Harness totalCents={150000} />);

    expect(within(row(1)).getByRole("button", { name: "Efectivo" })).toHaveAttribute("aria-pressed", "true");
    expect(amountOf(1)).toHaveValue("1,500.00");

    rerender(<Harness totalCents={90000} />);
    expect(amountOf(1)).toHaveValue("900.00");
  });

  it("stops following the amount once the cashier types another", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<Harness totalCents={150000} />);

    await user.clear(amountOf(1));
    await user.type(amountOf(1), "500");
    rerender(<Harness totalCents={90000} />);

    expect(amountOf(1)).toHaveValue("500");
  });

  it("shows the total, what is covered and what is left, all the time", async () => {
    const user = userEvent.setup();
    render(<Harness totalCents={150000} />);

    expect(screen.getByTestId("tender-total")).toHaveTextContent("$1,500.00");
    expect(screen.getByTestId("tender-covered")).toHaveTextContent("$1,500.00");
    expect(screen.getByTestId("tender-remaining")).toHaveTextContent("$0.00");

    await user.clear(amountOf(1));
    await user.type(amountOf(1), "500");
    expect(screen.getByTestId("tender-covered")).toHaveTextContent("$500.00");
    expect(screen.getByTestId("tender-remaining")).toHaveTextContent("$1,000.00");

    await user.clear(amountOf(1));
    await user.type(amountOf(1), "1600");
    expect(screen.getByTestId("tender-remaining")).toHaveTextContent("excede por $100.00");
  });

  // The case the school asked for: a credit note that does not cover it all,
  // completed with another method in one click.
  it("completes a credit note with another method for what is left", async () => {
    const user = userEvent.setup();
    const onValid = vi.fn();
    render(<Harness totalCents={150000} onValid={onValid} />);

    await user.click(within(row(1)).getByRole("button", { name: "Nota de crédito" }));
    await user.clear(amountOf(1));
    await user.type(amountOf(1), "500");
    await user.type(within(row(1)).getByLabelText("Referencia"), "NC-12");
    await user.click(screen.getByRole("button", { name: "Agregar método" }));

    expect(within(row(2)).getByRole("button", { name: "Efectivo" })).toHaveAttribute("aria-pressed", "true");
    expect(amountOf(2)).toHaveValue("1,000.00");
    expect(screen.getByTestId("tender-remaining")).toHaveTextContent("$0.00");

    await user.click(screen.getByRole("button", { name: "Guardar" }));
    expect(onValid).toHaveBeenCalledWith([
      { method: "credit_note", amountCents: 50000, reference: "NC-12" },
      { method: "cash", amountCents: 100000 },
    ]);
  });

  it("accepts a credit note alone for the whole amount", async () => {
    const user = userEvent.setup();
    const onValid = vi.fn();
    render(<Harness totalCents={150000} onValid={onValid} />);

    await user.click(within(row(1)).getByRole("button", { name: "Nota de crédito" }));
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(onValid).toHaveBeenCalledWith([{ method: "credit_note", amountCents: 150000 }]);
  });

  it("refuses a split that does not add up, on the last amount, and saves nothing", async () => {
    const user = userEvent.setup();
    const onValid = vi.fn();
    render(<Harness totalCents={150000} onValid={onValid} />);

    await user.clear(amountOf(1));
    await user.type(amountOf(1), "500");
    await user.click(screen.getByRole("button", { name: "Agregar método" }));
    await user.clear(amountOf(2));
    await user.type(amountOf(2), "900");
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(onValid).not.toHaveBeenCalled();
    expect(within(row(2)).getByRole("alert")).toHaveTextContent("La suma de las formas de pago debe ser igual al monto a pagar");
    expect(amountOf(2)).toHaveAttribute("aria-invalid", "true");

    // Fixed as it is typed.
    await user.clear(amountOf(2));
    await user.type(amountOf(2), "1000");
    expect(within(row(2)).queryByRole("alert")).toBeNull();
  });

  it("says an empty amount is wrong when the field is left, not while it is typed", async () => {
    const user = userEvent.setup();
    render(<Harness totalCents={150000} />);

    await user.clear(amountOf(1));
    expect(within(row(1)).queryByRole("alert")).toBeNull();
    fireEvent.blur(amountOf(1));
    expect(within(row(1)).getByRole("alert")).toHaveTextContent("Escribe un monto mayor a cero.");
    await user.type(amountOf(1), "1");
    expect(within(row(1)).queryByRole("alert")).toBeNull();
  });

  it("removes a row, but never the last one, and stops at four", async () => {
    const user = userEvent.setup();
    render(<Harness totalCents={150000} />);

    expect(screen.queryByRole("button", { name: /Quitar forma de pago/ })).toBeNull();
    const add = (): Promise<void> => user.click(screen.getByRole("button", { name: "Agregar método" }));
    await add();
    await add();
    await add();
    expect(screen.getAllByRole("group", { name: /Forma de pago \d/ })).toHaveLength(4);
    expect(screen.queryByRole("button", { name: "Agregar método" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Quitar forma de pago 4" }));
    expect(screen.getAllByRole("group", { name: /Forma de pago \d/ })).toHaveLength(3);
  });

  it("asks which card only on a card, and a reference only when it is not cash", async () => {
    const user = userEvent.setup();
    const onValid = vi.fn();
    render(<Harness totalCents={150000} onValid={onValid} />);

    expect(within(row(1)).queryByLabelText("Referencia")).toBeNull();
    expect(within(row(1)).queryByRole("button", { name: "Crédito" })).toBeNull();

    await user.click(within(row(1)).getByRole("button", { name: "Tarjeta" }));
    await user.click(within(row(1)).getByRole("button", { name: "Crédito" }));
    expect(within(row(1)).getByLabelText("Referencia")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(onValid).toHaveBeenCalledWith([{ method: "card", amountCents: 150000, cardType: "credit" }]);
  });
});

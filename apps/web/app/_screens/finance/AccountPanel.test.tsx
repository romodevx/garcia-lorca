import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { AccountStatement } from "../../finance/api";
import type { Student } from "../../students/api";
import { Providers } from "../../test-utils";
import { AccountPanel } from "./AccountPanel";

/**
 * Pide el CAMPO y no su ayuda: el (?) de junto es un botón que se llama
 * "Ayuda sobre <campo>", así que una consulta laxa encuentra los dos.
 */
const CAMPO = { selector: "input, select, textarea" } as const;

const students = [
  { id: "stu-1", firstName: "Ana", lastName: "Ruiz" } as unknown as Student,
];

function statement(over: Partial<AccountStatement> = {}): AccountStatement {
  return {
    charges: [],
    payments: [],
    totalBalanceCents: 3200000,
    unallocatedCents: 0,
    dueTodayCents: 775000,
    surchargeCents: 15000,
    earlyDiscountCents: 10000,
    asOf: "2026-11-20",
    ...over,
  };
}

async function open(account: AccountStatement): Promise<void> {
  render(
    <Providers>
      <AccountPanel students={students} onLoad={async () => account} />
    </Providers>,
  );
  await userEvent.selectOptions(screen.getByLabelText(/alumno/i, CAMPO), "stu-1");
  await userEvent.click(screen.getByRole("button", { name: /estado de cuenta|cargar|ver/i }));
  await waitFor(() => expect(screen.getByTestId("due-today")).toBeInTheDocument());
}

describe("AccountPanel", () => {
  // The whole reason the field exists. The office quoted one number and the
  // parents' app showed another; the statement must lead with what is being
  // asked for TODAY, and keep the year's balance beside it rather than instead.
  it("encabeza con lo que se debe hoy, y deja el año al lado", async () => {
    await open(statement());

    expect(screen.getByTestId("due-today")).toHaveTextContent("7,750.00");
    expect(screen.getByTestId("year-balance")).toHaveTextContent("32,000.00");
  });

  // A family asking "why that number?" must be answered by the screen, not by a
  // phone call to the office.
  it("explica el recargo y el pronto pago que ya trae", async () => {
    await open(statement());

    const parts = screen.getByTestId("due-today-parts");
    expect(parts).toHaveTextContent(/150\.00/);
    expect(parts).toHaveTextContent(/100\.00/);
  });

  // These figures move overnight. A screen that cannot say which day it quotes
  // cannot be trusted the morning after.
  it("dice el día para el que está calculado", async () => {
    await open(statement());

    expect(screen.getByTestId("due-today-parts")).toHaveTextContent("2026-11-20");
  });

  // A gateway written before M18 sends no asOf. Showing $0.00 would tell a
  // family that owes money that they owe nothing.
  it("no inventa un cero cuando el servidor no mandó la cifra", async () => {
    await open(statement({ dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "" }));

    expect(screen.getByTestId("due-today")).not.toHaveTextContent("0.00");
    expect(screen.getByTestId("year-balance")).toHaveTextContent("32,000.00");
  });
});

import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { IncomePanel } from "./IncomePanel";

vi.mock("../../finance/api", () => ({ getIncome: vi.fn() }));
const finance = vi.mocked(financeApi);

beforeEach(() => {
  finance.getIncome.mockResolvedValue({
    lines: [
      { conceptId: "k1", concept: "Colegiatura", amountCents: 6400000 },
      { conceptId: "k2", concept: "Recargo por pago tardío", amountCents: 30000 },
    ],
    totalCents: 6430000,
    creditNoteCents: 0,
  });
});
afterEach(() => vi.clearAllMocks());

describe("IncomePanel", () => {
  it("leads with the period total and lists what earned it", async () => {
    render(<IncomePanel from="2026-10-01" to="2026-10-31" />);

    expect(await screen.findByTestId("income-total")).toHaveTextContent("64,300");
    expect(screen.getByText("Colegiatura")).toBeInTheDocument();
    expect(screen.getByText("Recargo por pago tardío")).toBeInTheDocument();
  });

  it("says so plainly when nothing came in", async () => {
    finance.getIncome.mockResolvedValue({ lines: [], totalCents: 0, creditNoteCents: 0 });

    render(<IncomePanel from="2026-10-01" to="2026-10-31" />);

    expect(await screen.findByText(/No se recibió nada/)).toBeInTheDocument();
  });
});

describe("IncomePanel · notas de crédito (tarea 8)", () => {
  it("shows the period's credit notes apart, outside the total", async () => {
    finance.getIncome.mockResolvedValue({ lines: [{ conceptId: "k1", concept: "Colegiatura", amountCents: 100000 }], totalCents: 100000, creditNoteCents: 50000 });

    render(<IncomePanel from="2026-10-01" to="2026-10-31" />);

    expect(await screen.findByTestId("income-total")).toHaveTextContent("1,000");
    expect(screen.getByTestId("income-credit-notes")).toHaveTextContent(/Notas de crédito.*\$500\.00/);
  });

  it("says nothing of credit notes when there were none", async () => {
    finance.getIncome.mockResolvedValue({ lines: [], totalCents: 0, creditNoteCents: 0 });
    render(<IncomePanel from="2026-10-01" to="2026-10-31" />);
    await screen.findByText(/No se recibió nada/);
    expect(screen.queryByTestId("income-credit-notes")).toBeNull();
  });
});

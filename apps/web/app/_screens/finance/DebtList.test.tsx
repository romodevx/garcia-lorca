import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { ExpectedLine } from "../../finance/api";
import { DebtList } from "./DebtList";

const line: ExpectedLine = {
  chargeId: "ch1", description: "Colegiatura septiembre", dueDate: "2026-09-10", owedCents: 160000,
  surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: 160000, due: true, inBalance: true,
};

function renderList(lines: ExpectedLine[]): void {
  const balance = lines.reduce((sum, l) => sum + l.totalCents, 0);
  render(<DebtList lines={lines} balanceCents={balance} onTimeCents={balance} chosen={new Set(["ch1"])} selectedCents={160000} onToggle={() => undefined} />);
}

describe("DebtList · scholarships (T18)", () => {
  it("says the scholarship is already out of what is owed", () => {
    renderList([{ ...line, scholarshipCents: 160000 }]);
    expect(screen.getByTestId("debt-row-ch1")).toHaveTextContent("$1,600.00 ya con beca −$1,600.00");
  });

  it("says nothing of a scholarship when none applies", () => {
    renderList([line]);
    expect(screen.getByTestId("debt-row-ch1")).not.toHaveTextContent(/beca/i);
  });
});

// D1 (T5 review): a charge not yet open is listed — it can be paid ahead — but
// it is not part of the debt. The list says so instead of leaving its rows to
// add up to more than "Total del adeudo".
describe("DebtList · lo que aún no abre (T5)", () => {
  const notOpen: ExpectedLine = {
    chargeId: "ch-ins", description: "Inscripción", dueDate: "2026-06-30", owedCents: 50000,
    surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: 50000, due: false, inBalance: false,
  };

  it("lists it apart, after the debt, and leaves it out of the total", () => {
    render(<DebtList lines={[notOpen, line]} balanceCents={160000} onTimeCents={160000} chosen={new Set()} selectedCents={0} onToggle={() => undefined} />);

    const rows = screen.getAllByTestId(/^debt-row-/).map((r) => r.dataset.testid);
    expect(rows).toEqual(["debt-row-ch1", "debt-row-ch-ins"]);
    const apart = screen.getByTestId("debt-not-open");
    expect(apart).toHaveTextContent(/Por abrir/);
    expect(apart).toHaveTextContent(/no suma al adeudo/i);
    expect(within(apart).getByTestId("debt-row-ch-ins")).toBeInTheDocument();
    expect(within(apart).queryByTestId("debt-row-ch1")).toBeNull();
    expect(screen.getByTestId("debt-total")).toHaveTextContent("$1,600.00");
  });

  it("shows no 'Por abrir' group when everything is open", () => {
    renderList([line]);
    expect(screen.queryByTestId("debt-not-open")).toBeNull();
  });

  // The on-time figure is the server's: the reward of a line not yet open
  // takes nothing off the debt, so the list never works it out itself.
  it("reads 'si paga a tiempo' from the server, never from the rows", () => {
    const rewarded: ExpectedLine = { ...line, discountCents: 20000, totalCents: 140000 };
    const aheadWithReward: ExpectedLine = { ...notOpen, discountCents: 30000, totalCents: 20000 };
    render(<DebtList lines={[rewarded, aheadWithReward]} balanceCents={160000} onTimeCents={140000} chosen={new Set()} selectedCents={0} onToggle={() => undefined} />);
    expect(screen.getByTestId("debt-on-time")).toHaveTextContent("$1,400.00");
  });
});

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { ExpectedLine } from "../../finance/api";
import { DebtList } from "./DebtList";

const line: ExpectedLine = {
  chargeId: "ch1", description: "Colegiatura septiembre", dueDate: "2026-09-10", owedCents: 160000,
  surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: 160000, due: true,
};

function renderList(lines: ExpectedLine[]): void {
  render(<DebtList lines={lines} chosen={new Set(["ch1"])} selectedCents={160000} onToggle={() => undefined} />);
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

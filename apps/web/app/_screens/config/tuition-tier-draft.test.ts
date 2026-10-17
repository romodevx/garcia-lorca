import { describe, expect, it } from "vitest";

import { readTierDraft } from "./tuition-math";

/**
 * ISSUE-013: what a concept card has typed and not "added", read the same way
 * by its "Agregar monto" button and by "Guardar tabulador".
 */
describe("readTierDraft", () => {
  const concept = "INSCRIPCIÓN";

  it("nothing typed is nothing pending", () => {
    expect(readTierDraft(concept, { amount: " ", from: "", to: "" })).toEqual({ kind: "empty" });
  });

  it("only the date the card opened with is still nothing pending (ISSUE-T14-007)", () => {
    expect(readTierDraft(concept, { amount: "", from: "2026-08-01", to: "" }, "2026-08-01")).toEqual({ kind: "empty" });
  });

  it("names the field a half-filled draft lacks, so the save can focus it", () => {
    expect(readTierDraft(concept, { amount: "4000", from: "", to: "" })).toMatchObject({ kind: "problem", field: "from" });
    expect(readTierDraft(concept, { amount: "", from: "2026-09-01", to: "" })).toMatchObject({ kind: "problem", field: "amount" });
  });

  it("a filled amount and start date is a tier, in cents", () => {
    expect(readTierDraft(concept, { amount: "4,500.00", from: "2026-08-01", to: "" })).toEqual({
      kind: "tier",
      tier: { effectiveFrom: "2026-08-01", effectiveTo: "", amountCents: 450000 },
    });
  });

  it.each([
    { draft: { amount: "4500", from: "", to: "" }, says: /INSCRIPCIÓN.*Vigente desde/ },
    { draft: { amount: "", from: "2026-08-01", to: "" }, says: /INSCRIPCIÓN.*falta el monto/ },
    { draft: { amount: "", from: "", to: "2026-09-30" }, says: /falta el monto/ },
    { draft: { amount: "45oo", from: "2026-08-01", to: "" }, says: /no se entiende/ },
    { draft: { amount: "0", from: "2026-08-01", to: "" }, says: /mayor a cero/ },
  ])("a half-filled draft is a problem that says what is missing ($draft)", ({ draft, says }) => {
    const read = readTierDraft(concept, draft);
    expect(read.kind).toBe("problem");
    expect(read.kind === "problem" ? read.message : "").toMatch(says);
  });
});

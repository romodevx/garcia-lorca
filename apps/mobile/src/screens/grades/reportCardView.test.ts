import { reportCardLabel, reportCardTitle, versionBadge } from "./reportCardView";

const card = { id: "rc2", termId: "t2", termName: "2.º trimestre", termOrdinal: 2, cycleName: "2026-2027", version: 2, issuedAt: "2027-03-20", generalAverage: 8.75, hasGrades: true };

describe("reportCardView", () => {
  it("titles a boleta with its term and cycle, or the term alone", () => {
    expect(reportCardTitle(card)).toBe("2.º trimestre · 2026-2027");
    expect(reportCardTitle({ ...card, cycleName: "" })).toBe("2.º trimestre");
  });

  it("badges only corrected versions", () => {
    expect(versionBadge(1)).toBeNull();
    expect(versionBadge(3)).toBe("Versión 3 · Actualizada");
  });

  it("reads the whole row aloud", () => {
    expect(reportCardLabel(card)).toBe("2.º trimestre 2026-2027, versión 2, actualizada, promedio general 8.75, emitida el 20 de marzo");
    expect(reportCardLabel({ ...card, version: 1, generalAverage: null, hasGrades: false, issuedAt: null })).toBe(
      "2.º trimestre 2026-2027, sin calificaciones",
    );
  });
});

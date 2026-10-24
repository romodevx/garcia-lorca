import { describe, expect, it } from "vitest";

import type { StudentImportFamilies } from "./api";
import { builtFamilyNames, canUndoFamilies, familiesHeadline, reviewLinkText, undoOutcome } from "./families";

const fam = (id: string, name?: string) => ({ id, ...(name === undefined ? {} : { name }) });
const built = (over: Partial<StudentImportFamilies> = {}): StudentImportFamilies => ({ built: true, created: [], joined: [], skipped: 0, ...over });

describe("the import's families, in words", () => {
  it("counts the families it built, singular and plural", () => {
    expect(familiesHeadline(built({ created: [fam("f1")] }))).toBe("Armamos 1 familia automáticamente.");
    expect(familiesHeadline(built({ created: [fam("f1"), fam("f2"), fam("f3")] }))).toBe("Armamos 3 familias automáticamente.");
  });

  it("adds the existing families pupils joined", () => {
    expect(familiesHeadline(built({ created: [fam("f1"), fam("f2")], joined: [fam("f3")] })))
      .toBe("Armamos 2 familias automáticamente y sumamos alumnos a 1 familia que ya existía.");
    expect(familiesHeadline(built({ joined: [fam("f3"), fam("f4")] }))).toBe("Sumamos alumnos a 2 familias que ya existían.");
  });

  it("says when nothing was very likely", () => {
    expect(familiesHeadline(built())).toBe("No encontramos familias muy probables.");
  });

  it("any incomplete build reads as unconfirmed, even with nothing listed", () => {
    const line = "No pudimos confirmar todas las familias; lo que no se armó queda como sugerencia.";
    expect(familiesHeadline(built({ built: false }))).toBe(line);
    expect(familiesHeadline(built({ built: false, created: [fam("f1")] }))).toBe(line);
  });

  it("offers undo whenever something may have been built", () => {
    expect(canUndoFamilies(built({ created: [fam("f1")] }))).toBe(true);
    expect(canUndoFamilies(built({ joined: [fam("f1")] }))).toBe(true);
    expect(canUndoFamilies(built({ built: false }))).toBe(true);
    expect(canUndoFamilies(built())).toBe(false);
  });

  it("lists the names it knows, created first", () => {
    expect(builtFamilyNames(built({ created: [fam("f1", "GARCÍA TORRES"), fam("f2")], joined: [fam("f3", "RUIZ PAZ")] }))).toEqual(["GARCÍA TORRES", "RUIZ PAZ"]);
  });

  it("words the review link", () => {
    expect(reviewLinkText(1)).toBe("1 posible familia por revisar");
    expect(reviewLinkText(4)).toBe("4 posibles familias por revisar");
  });

  it("words what the undo did", () => {
    expect(undoOutcome({ removed: 5, dissolved: 2, keptWithAccess: 0 })).toEqual(["Deshicimos 2 familias."]);
    expect(undoOutcome({ removed: 2, dissolved: 1, keptWithAccess: 0 })).toEqual(["Deshicimos 1 familia."]);
    expect(undoOutcome({ removed: 3, dissolved: 0, keptWithAccess: 0 })).toEqual(["Sacamos a 3 alumnos de las familias a las que se sumaron."]);
    expect(undoOutcome({ removed: 0, dissolved: 0, keptWithAccess: 0 })).toEqual(["Ya no quedaba nada por deshacer."]);
    expect(undoOutcome({ removed: 4, dissolved: 2, keptWithAccess: 1 })).toEqual([
      "Deshicimos 2 familias.",
      "1 familia ya tiene acceso a la app y no se tocó; quita el acceso desde la familia primero.",
    ]);
    expect(undoOutcome({ removed: 0, dissolved: 0, keptWithAccess: 2 })).toEqual([
      "2 familias ya tienen acceso a la app y no se tocaron; quita el acceso desde la familia primero.",
    ]);
  });
});

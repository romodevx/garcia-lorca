import { describe, expect, it } from "vitest";

import { StudentImportError, type StudentImportFamilies } from "./api";
import { UNDONE_HEADLINE, builtFamilyNames, canUndoFamilies, familiesHeadline, reviewLinkText, undoFailureText, undoOutcome } from "./families";

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

  it("lists the families whose names it knows, created first", () => {
    expect(builtFamilyNames(built({ created: [fam("f1", "GARCÍA TORRES"), fam("f2")], joined: [fam("f3", "RUIZ PAZ")] }))).toEqual([
      { id: "f1", name: "GARCÍA TORRES" },
      { id: "f3", name: "RUIZ PAZ" },
    ]);
  });

  it("after a complete undo the headline says so", () => {
    expect(UNDONE_HEADLINE).toBe("Deshicimos las familias que armamos.");
  });

  it.each([
    [404, "Esta función ya no está disponible; recarga la página."],
    [403, "Tu cuenta no tiene permiso para deshacer familias."],
    [429, "Demasiados intentos seguidos. Espera un momento y vuelve a intentar."],
    [500, "No pudimos deshacer las familias. Intenta de nuevo."],
    [400, "No pudimos deshacer las familias. Intenta de nuevo."],
  ])("a %i on undo reads «%s»", (status, line) => {
    expect(undoFailureText(new StudentImportError(status, "x", undefined))).toBe(line);
  });

  it("an undo that never got an answer keeps the generic line", () => {
    expect(undoFailureText(new TypeError("Failed to fetch"))).toBe("No pudimos deshacer las familias. Intenta de nuevo.");
  });

  it("words the review link", () => {
    expect(reviewLinkText(1)).toBe("1 posible familia por revisar");
    expect(reviewLinkText(4)).toBe("4 posibles familias por revisar");
  });

  it("words what the undo did from its two counts, accurately in every mix", () => {
    expect(undoOutcome({ removed: 4, dissolved: 2, keptWithAccess: 0 })).toEqual(["Sacamos a 4 alumnos de sus familias y deshicimos 2 familias que quedaron vacías."]);
    expect(undoOutcome({ removed: 2, dissolved: 1, keptWithAccess: 0 })).toEqual(["Sacamos a 2 alumnos de sus familias y deshicimos 1 familia que quedó vacía."]);
    expect(undoOutcome({ removed: 3, dissolved: 0, keptWithAccess: 0 })).toEqual(["Sacamos a 3 alumnos de sus familias."]);
    expect(undoOutcome({ removed: 1, dissolved: 0, keptWithAccess: 0 })).toEqual(["Sacamos a 1 alumno de su familia."]);
    expect(undoOutcome({ removed: 0, dissolved: 0, keptWithAccess: 0 })).toEqual(["Ya no quedaba nada por deshacer."]);
  });

  it("names a path that exists for the families it left whole", () => {
    expect(undoOutcome({ removed: 4, dissolved: 2, keptWithAccess: 1 })).toEqual([
      "Sacamos a 4 alumnos de sus familias y deshicimos 2 familias que quedaron vacías.",
      "1 familia ya tiene acceso a la app y no se tocó: para deshacerla, quita el acceso y saca a los alumnos desde Alumnos › Familias.",
    ]);
    expect(undoOutcome({ removed: 0, dissolved: 0, keptWithAccess: 2 })).toEqual([
      "2 familias ya tienen acceso a la app y no se tocaron: para deshacerlas, quita el acceso y saca a los alumnos desde Alumnos › Familias.",
    ]);
  });
});

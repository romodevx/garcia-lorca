import { describe, expect, it } from "vitest";

import { fold } from "./fold";

/**
 * Accent/diacritic folding is the #1 thing that breaks Spanish search: a user
 * typing `maria` must hit `María`, `nino` must hit `niño`. `fold` normalizes a
 * string to a lowercase, diacritic-free, trimmed form used on BOTH the query and
 * the haystack before matching.
 */
describe("fold", () => {
  it("strips accents from vowels", () => {
    expect(fold("María")).toBe("maria");
    expect(fold("José Pérez")).toBe("jose perez");
    expect(fold("Calificación")).toBe("calificacion");
  });

  it("folds ñ to n (Spanish search convenience)", () => {
    expect(fold("niño")).toBe("nino");
    expect(fold("Peña")).toBe("pena");
  });

  it("lowercases and trims", () => {
    expect(fold("  ESTUDIANTES  ")).toBe("estudiantes");
  });

  it("collapses internal whitespace runs to single spaces", () => {
    expect(fold("crear    alumno")).toBe("crear alumno");
  });

  it("is idempotent", () => {
    expect(fold(fold("Bécàs Ñoño"))).toBe(fold("Bécàs Ñoño"));
  });

  it("leaves already-plain ascii untouched", () => {
    expect(fold("crear alumno")).toBe("crear alumno");
  });
});

import { describe, expect, it } from "vitest";

import { checkParity, type ParityIssue, type RawCatalog } from "./parity";

/**
 * The parity validator is the safety net that keeps the bilingual catalog
 * honest. Key-only parity is NOT enough: a key can exist in both languages yet
 * crash at render time when its ICU argument signature diverges. These tests
 * pin every failure mode the validator must catch.
 *
 *   es: { "greet": "Hola {name}" }     en: { "greet": "Hello" }
 *        └────────── same key ─────────────┘   ← passes a key-only check
 *        └ {name} ┘            └ (none) ┘       ← diverges → runtime break
 */

const kinds = (issues: ParityIssue[]): string[] => issues.map((i) => i.kind);

function run(es: RawCatalog, en: RawCatalog): ParityIssue[] {
  return checkParity("common", { es, en });
}

describe("checkParity", () => {
  it("returns no issues for catalogs that fully match", () => {
    const es: RawCatalog = { "button.save": "Guardar", "items.count": "{count, plural, one {# elemento} other {# elementos}}" };
    const en: RawCatalog = { "button.save": "Save", "items.count": "{count, plural, one {# item} other {# items}}" };
    expect(run(es, en)).toEqual([]);
  });

  it("flags a key present in es but missing in en (and vice versa)", () => {
    const missingInEn = run({ "a": "A", "b": "B" }, { "a": "A" });
    expect(kinds(missingInEn)).toContain("missing-key");
    expect(missingInEn.find((i) => i.kind === "missing-key")?.key).toBe("b");

    const missingInEs = run({ "a": "A" }, { "a": "A", "z": "Z" });
    expect(missingInEs.find((i) => i.kind === "missing-key")?.key).toBe("z");
  });

  it("flags divergent ICU argument signatures (placeholder present in one language only)", () => {
    const issues = run({ "greet": "Hola {name}" }, { "greet": "Hello" });
    expect(kinds(issues)).toContain("arg-mismatch");
  });

  it("flags an argument that is a plural in es but a bare argument in en", () => {
    const issues = run(
      { "n": "{count, plural, one {uno} other {#}}" },
      { "n": "{count}" },
    );
    expect(kinds(issues)).toContain("arg-mismatch");
  });

  it("flags a message that does not parse as ICU", () => {
    const issues = run({ "broken": "{count, plural, one {uno}" }, { "broken": "ok" });
    expect(kinds(issues)).toContain("parse-error");
  });

  it("flags a plural without an `other` branch (ICU requires it)", () => {
    const issues = run(
      { "n": "{count, plural, one {uno}}" },
      { "n": "{count, plural, one {one}}" },
    );
    expect(kinds(issues)).toContain("missing-other");
  });

  it("flags a select without an `other` branch", () => {
    const issues = run(
      { "g": "{gender, select, female {ella}}" },
      { "g": "{gender, select, female {she}}" },
    );
    expect(kinds(issues)).toContain("missing-other");
  });

  it("flags empty or whitespace-only values", () => {
    expect(kinds(run({ "x": "" }, { "x": "y" }))).toContain("empty-value");
    expect(kinds(run({ "x": "   " }, { "x": "y" }))).toContain("empty-value");
  });

  it("flags a leaf/branch collision — a key that is also a prefix of another key", () => {
    // 'a' is a leaf AND the prefix of 'a.b'. This is legal flat JSON but breaks
    // the nested transform both next-intl and react-i18next require.
    const issues = checkParity("common", { es: { "a": "x", "a.b": "y" }, en: { "a": "x", "a.b": "y" } });
    expect(kinds(issues)).toContain("key-collision");
    expect(issues.find((i) => i.kind === "key-collision")?.key).toBe("a");
  });

  it("flags nested (non-flat) keys — values must be strings", () => {
    const es = { "group": { "child": "no" } } as unknown as RawCatalog;
    const issues = checkParity("common", { es, en: { "group.child": "no" } });
    expect(kinds(issues)).toContain("nested-key");
  });

  it("reports the namespace and key on every issue", () => {
    const issues = checkParity("auth", { es: { "email": "" }, en: { "email": "Email" } });
    const issue = issues.find((i) => i.kind === "empty-value");
    expect(issue?.namespace).toBe("auth");
    expect(issue?.key).toBe("email");
  });
});

import { describe, expect, it } from "vitest";

import { toNested } from "./nest";

/**
 * Both next-intl and react-i18next treat "." as a nesting separator and cannot
 * consume flat keys with literal dots (confirmed in next-intl docs). The package
 * keeps catalogs FLAT (type-safe via `keyof typeof`, simple parity) and exposes
 * this framework-agnostic transform so each binding feeds its engine a nested tree.
 */
describe("toNested", () => {
  it("expands dot-separated keys into a nested tree", () => {
    expect(toNested({ "a.b": "1", "a.c": "2", "x": "3" })).toEqual({
      a: { b: "1", c: "2" },
      x: "3",
    });
  });

  it("preserves ICU message strings verbatim at the leaves", () => {
    const icu = "{count, plural, one {# alumno} other {# alumnos}}";
    expect(toNested({ "label.count": icu })).toEqual({ label: { count: icu } });
  });

  it("handles a single-segment key", () => {
    expect(toNested({ greeting: "Hola" })).toEqual({ greeting: "Hola" });
  });

  it("throws on a leaf/branch collision rather than silently dropping a message", () => {
    expect(() => toNested({ "a": "leaf", "a.b": "branch" })).toThrow(/collision/i);
  });
});

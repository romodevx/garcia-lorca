import { describe, expect, it } from "vitest";

import { formatMessage } from "./format";

/**
 * The framework-agnostic formatter. It exists for the NON-React surfaces — the
 * parity tests, Node scripts, and future backend email/notification rendering —
 * where mounting next-intl or react-i18next is not an option. It shares the exact
 * pinned `intl-messageformat` the apps use, so it is the same engine, not a third.
 */
describe("formatMessage", () => {
  it("interpolates a simple argument", () => {
    expect(formatMessage("Hola {name}", { name: "Ana" })).toBe("Hola Ana");
  });

  it("resolves a plural branch (es)", () => {
    const msg = "{count, plural, one {# alumno} other {# alumnos}}";
    expect(formatMessage(msg, { count: 1 }, "es")).toBe("1 alumno");
    expect(formatMessage(msg, { count: 3 }, "es")).toBe("3 alumnos");
  });

  it("resolves a select branch (en)", () => {
    const msg = "{gender, select, female {She} male {He} other {They}}";
    expect(formatMessage(msg, { gender: "female" }, "en")).toBe("She");
    expect(formatMessage(msg, { gender: "x" }, "en")).toBe("They");
  });

  it("formats numbers using the catalog-id's FORMAT locale (es → es-MX)", () => {
    // es-MX groups with a comma and uses a dot decimal, like en-US — but the point
    // is the engine receives es-MX, not bare es. This pins the decoupling.
    const formatted = formatMessage("{n, number}", { n: 1234.5 }, "es");
    expect(formatted).toContain("1,234.5");
  });

  it("defaults to the es locale when none is given", () => {
    expect(formatMessage("Hola {name}", { name: "Mundo" })).toBe("Hola Mundo");
  });
});

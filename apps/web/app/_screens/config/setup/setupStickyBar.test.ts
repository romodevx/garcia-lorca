import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

/*
 * Below 900 px the "Paso N de 8" bar sticks right under the top bar (§1.6,
 * WCAG 2.2 · 2.4.11). Its `top` must be the top bar's real height, or it
 * slides under the bar (or leaves a gap). The bar publishes that height once,
 * as --topbar-h (PLAN-redesign-home F2a, review E6), and the setup layer reads
 * it instead of keeping its own copy of the number.
 */

const css = readFileSync("app/globals.css", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");

describe("setup sticky step bar (design review N9)", () => {
  it("the top bar publishes its height as --topbar-h and uses it", () => {
    expect(css).toMatch(/:root\s*\{[^}]*--topbar-h:\s*78px;/);
    expect(css).toMatch(/\.topbar\s*\{[^}]*height:\s*var\(--topbar-h\);/);
  });

  it("every --setup-topbar-h is the top bar's height, not a copy of the number", () => {
    const declared = [...css.matchAll(/--setup-topbar-h:\s*([^;]+);/g)].map((m) => m[1]?.trim());
    expect(declared.length).toBeGreaterThan(0);
    expect(declared.every((v) => v === "var(--topbar-h)")).toBe(true);
  });

  it("no rule changes the top bar's height but the token (a phone included)", () => {
    const topbarBlocks = [...css.matchAll(/(^|[\s,}])\.topbar\s*\{([^}]*)\}/g)].map((m) => m[2] ?? "");
    const heights = topbarBlocks.flatMap((b) => [...b.matchAll(/(^|[;\s])height:\s*([^;]+);/g)].map((m) => m[2]?.trim()));
    expect(heights).toEqual(["var(--topbar-h)"]);
  });
});

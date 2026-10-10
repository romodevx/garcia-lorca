import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

/*
 * Below 900 px the "Paso N de 8" bar sticks right under the top bar (§1.6,
 * WCAG 2.2 · 2.4.11). The top bar's height is an inline style in TopBar.tsx,
 * and an inline style beats every @media rule: it is 78 px on a phone too. So
 * the bar's `top` must never assume less, or it slides under the top bar.
 */

const css = readFileSync("app/globals.css", "utf8");
const topBar = readFileSync("app/_design/TopBar.tsx", "utf8");

describe("setup sticky step bar (design review N9)", () => {
  it("every --setup-topbar-h is the top bar's real height", () => {
    const inlineHeight = Number(/height:\s*(\d+),/.exec(topBar)?.[1]);
    const declared = [...css.matchAll(/--setup-topbar-h:\s*(\d+)px/g)].map((m) => Number(m[1]));
    expect(declared.length).toBeGreaterThan(0);
    expect(declared.every((px) => px === inlineHeight)).toBe(true);
  });
});

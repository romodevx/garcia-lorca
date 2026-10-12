import { readFileSync, globSync } from "node:fs";
import { render } from "@testing-library/react";
import type { CSSProperties } from "react";
import { describe, expect, it } from "vitest";
import { Icon } from "./Icon";
import { ModuleHeaderIcon, type ModuleHeaderIconProps } from "./ModuleHeaderIcon";
import { DESKTOP, resolvedStyle } from "./testing/declaredStyle";

/*
 * PLAN-redesign-home F2a (review D3/E10): twenty module screens drew the same
 * 56px header square inline. One component replaces them, so F2b can restyle
 * every header from one class. The table below is each site's inline style,
 * copied verbatim before it was deleted, next to the props that replace it:
 * the two must resolve to the same declared values.
 */

const SQUARE = { width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center" } as const;
const glow = (c: string, pct: number): string => `0 6px 16px color-mix(in oklab, ${c} ${pct}%, transparent)`;
const solid = (c: string, ink: string, pct: number): CSSProperties => ({ ...SQUARE, background: c, color: ink, boxShadow: glow(c, pct) });
const NEUTRAL: CSSProperties = { ...SQUARE, background: "var(--surface-3)", color: "var(--text-muted)" };

const SITES: Array<[site: string, legacy: CSSProperties, props: Omit<ModuleHeaderIconProps, "icon">]> = [
  ["AttendanceHeader", { ...solid("var(--lime)", "var(--primary-ink)", 40), flexShrink: 0 }, { color: "lime", ink: "var(--primary-ink)" }],
  ["TeacherCapture / DirectorCapture", { ...solid("var(--yellow)", "#3A2E00", 40), flexShrink: 0 }, { color: "yellow" }],
  ["Schedule / Expenses / Activities", { ...solid("var(--forest)", "#fff", 38), flexShrink: 0 }, { color: "forest" }],
  ["Payments / Catalog / SetupHeader / AdminScreen / Students", solid("var(--forest)", "#fff", 38), { color: "forest", shrinks: true }],
  ["Announcements / Finance", solid("var(--orange)", "#fff", 38), { color: "orange", shrinks: true }],
  ["Teachers", solid("var(--purple)", "#fff", 38), { color: "purple", shrinks: true }],
  ["Uniforms / Scholarships", { ...solid("var(--purple)", "#fff", 38), flexShrink: 0 }, { color: "purple" }],
  ["TeacherLoad", { ...SQUARE, flexShrink: 0, background: "var(--purple)", color: "#fff" }, { color: "purple", glow: false }],
  ["Admin / Enrollment", NEUTRAL, { color: "neutral", shrinks: true }],
  ["ModulePlaceholder (lime)", { ...SQUARE, background: "var(--lime)", color: "#0C3318" }, { color: "lime", glow: false, shrinks: true }],
  ["ModulePlaceholder (orange)", { ...SQUARE, background: "var(--orange)", color: "#FFFFFF" }, { color: "orange", glow: false, shrinks: true }],
  ["ModulePlaceholder (neutral)", NEUTRAL, { color: "neutral", glow: false, shrinks: true }]
];

/** A property set to its initial value draws exactly what leaving it out draws. */
const INITIAL: Record<string, string> = { "flex-shrink": "1", "box-shadow": "none" };
function drawn(el: Element): Record<string, string> {
  const style = resolvedStyle(el, DESKTOP);
  return Object.fromEntries(
    Object.entries(style)
      .filter(([prop, value]) => INITIAL[prop] !== value)
      .map(([prop, value]) => [prop, sameHex(value)])
  );
}

/** #fff and #FFFFFF are one color: the screens wrote the short form, COLOR the long one. */
function sameHex(value: string): string {
  return value
    .replace(/#([0-9a-f])([0-9a-f])([0-9a-f])\b/gi, "#$1$1$2$2$3$3")
    .replace(/#[0-9a-f]{6}\b/gi, (hex) => hex.toUpperCase());
}

function root(container: HTMLElement): Element {
  const el = container.firstElementChild;
  if (el === null) throw new Error("nothing rendered");
  return el;
}

describe("ModuleHeaderIcon", () => {
  it.each(SITES)("%s: draws what its inline square drew", (_site, legacy, props) => {
    const before = root(render(<div style={legacy}><Icon name="settings" size={30} /></div>).container);
    const after = root(render(<ModuleHeaderIcon icon="settings" {...props} />).container);
    expect(drawn(after)).toEqual(drawn(before));
    expect(after.innerHTML).toBe(before.innerHTML);
  });

  it("keeps only custom properties inline, so the stylesheet owns the look", () => {
    const el = root(render(<ModuleHeaderIcon icon="qr" color="lime" />).container) as HTMLElement;
    expect([...el.style].filter((p) => !p.startsWith("--"))).toEqual([]);
  });

  // The CounterSalePanel square is a "sale recorded" check, not a header.
  const NOT_HEADERS = new Set(["app/_screens/uniforms/CounterSalePanel.tsx"]);
  it("no screen draws its own 56px header square any more", () => {
    const copies = globSync("app/**/*.tsx")
      .filter((f) => !f.includes(".test.") && !NOT_HEADERS.has(f))
      .filter((f) => /width:\s*56,\s*height:\s*56,[^}]*borderRadius:\s*18/.test(readFileSync(f, "utf8")));
    expect(copies).toEqual([]);
  });
});

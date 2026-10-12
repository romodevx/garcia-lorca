import { readFileSync } from "node:fs";

/*
 * A small, honest cascade over globals.css + React inline styles, for
 * characterization tests that must prove "moving a style from inline to a
 * class changed nothing".
 *
 * Why not jsdom's getComputedStyle: in jsdom 29 it does not substitute var(),
 * it mangles any shorthand that contains var() (`border: 1px solid var(--x)`
 * reads back as `medium none rgb(0, 0, 0)` — on BOTH sides, so a wrong border
 * would still compare equal), it ignores @media, and it never matches :hover.
 * Every one of those is exactly what this refactor touches.
 *
 * What this does instead, per element:
 *   1. every rule of globals.css whose selector matches (el.matches), whose
 *      @media holds for the given viewport width, ordered by specificity and
 *      source order;
 *   2. then the element's inline style, read as the exact object React was
 *      given (not the mangled DOM attribute), with React's own px rule;
 *   3. then !important rules;
 *   4. then var() substituted from the custom properties declared on the
 *      element or its nearest ancestor (:root included).
 *
 * Known approximation: var() is substituted at the element being read, not
 * where each custom property was declared. No style in this app depends on the
 * difference.
 */

type Declaration = { prop: string; value: string; important: boolean };
type StyleRule = { selectors: string[]; media: string | null; declarations: Declaration[]; order: number };

export type StyleContext = {
  /** Viewport width in CSS px, for @media (min-|max-width). */
  width: number;
};

export const DESKTOP: StyleContext = { width: 1280 };
export const PHONE: StyleContext = { width: 400 };

const HOVER_ATTR = "data-declared-hover";

function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

/** Index of the `}` closing the block whose `{` is at `open`. */
function closingBrace(css: string, open: number): number {
  let depth = 0;
  for (let i = open; i < css.length; i++) {
    if (css[i] === "{") depth++;
    if (css[i] === "}" && --depth === 0) return i;
  }
  return css.length;
}

/** Splits on `sep` outside parentheses and quotes. */
export function splitTopLevel(text: string, sep: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let quote: string | null = null;
  let start = 0;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (quote !== null) {
      if (ch === quote) quote = null;
      continue;
    }
    if (ch === "\"" || ch === "'") quote = ch;
    else if (ch === "(") depth++;
    else if (ch === ")") depth--;
    else if (ch === sep && depth === 0) {
      parts.push(text.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(text.slice(start));
  return parts.map((p) => p.trim()).filter((p) => p !== "");
}

function parseDeclarations(body: string): Declaration[] {
  return splitTopLevel(body, ";").flatMap((raw) => {
    const colon = raw.indexOf(":");
    if (colon < 0) return [];
    const prop = raw.slice(0, colon).trim();
    const rawValue = raw.slice(colon + 1).trim();
    const important = /!important$/.test(rawValue);
    const value = normalize(rawValue.replace(/\s*!important$/, ""));
    return [{ prop: prop.startsWith("--") ? prop : prop.toLowerCase(), value, important }];
  });
}

function parseBlock(css: string, media: string | null, out: StyleRule[]): void {
  let i = 0;
  while (i < css.length) {
    const open = css.indexOf("{", i);
    if (open < 0) return;
    const prelude = css.slice(i, open).trim();
    const close = closingBrace(css, open);
    const body = css.slice(open + 1, close);
    if (prelude.startsWith("@media")) {
      const params = prelude.slice("@media".length).trim();
      parseBlock(body, media === null ? params : `${media} and ${params}`, out);
    } else if (!prelude.startsWith("@")) {
      out.push({ selectors: splitTopLevel(prelude, ","), media, declarations: parseDeclarations(body), order: out.length });
    }
    i = close + 1;
  }
}

export function parseStylesheet(css: string): StyleRule[] {
  const rules: StyleRule[] = [];
  parseBlock(stripComments(css), null, rules);
  return rules;
}

function featureHolds(feature: string, ctx: StyleContext): boolean {
  const f = feature.trim().replace(/^\(|\)$/g, "").trim();
  const minW = /^min-width:\s*(\d+)px$/.exec(f);
  if (minW !== null) return ctx.width >= Number(minW[1]);
  const maxW = /^max-width:\s*(\d+)px$/.exec(f);
  if (maxW !== null) return ctx.width <= Number(maxW[1]);
  // screen, all: yes. print, reduced motion, dark scheme, coarse pointers,
  // forced colors: the default desktop session says no.
  return f === "screen" || f === "all";
}

function mediaHolds(media: string | null, ctx: StyleContext): boolean {
  if (media === null) return true;
  return splitTopLevel(media, ",").some((alt) => alt.split(/\s+and\s+/).every((f) => featureHolds(f, ctx)));
}

/** [ids, classes+attributes+pseudo-classes, types] — enough for this sheet. */
function specificity(selector: string): [number, number, number] {
  const s = selector.replace(/::[\w-]+(\([^)]*\))?/g, " ");
  const ids = (s.match(/#[\w-]+/g) ?? []).length;
  const classes = (s.match(/\.[\w-]+|\[[^\]]+\]|:(?!where)[\w-]+/g) ?? []).length;
  const types = (s.replace(/\[[^\]]+\]|\([^)]*\)/g, "").match(/(^|[\s>+~])[a-z][\w-]*/gi) ?? []).length;
  return [ids, classes, types];
}

function compareSpecificity(a: [number, number, number], b: [number, number, number]): number {
  return a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
}

function safeMatches(el: Element, selector: string): boolean {
  try {
    return el.matches(selector.replace(/:hover/g, `[${HOVER_ATTR}]`));
  } catch {
    return false; // pseudo-elements and selectors jsdom cannot evaluate
  }
}

const UNITLESS = new Set([
  "animationIterationCount", "flex", "flexGrow", "flexShrink", "fontWeight", "lineHeight",
  "opacity", "order", "zIndex", "zoom", "gridRow", "gridColumn", "aspectRatio"
]);

function kebab(prop: string): string {
  if (prop.startsWith("--")) return prop;
  const k = prop.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);
  return /^(webkit|moz|ms)-/.test(k) ? `-${k}` : k;
}

function normalize(value: string): string {
  return value.replace(/\s+/g, " ").replace(/\(\s+/g, "(").replace(/\s+\)/g, ")").trim();
}

/** The exact inline style object React rendered, keyed in CSS (kebab) form. */
function inlineDeclarations(el: Element): Declaration[] {
  const key = Object.keys(el).find((k) => k.startsWith("__reactProps$"));
  if (key === undefined) return [];
  const props = (el as unknown as Record<string, { style?: Record<string, unknown> }>)[key];
  const style = props?.style ?? {};
  return Object.entries(style).flatMap(([prop, raw]) => {
    if (raw === undefined || raw === null || raw === "") return [];
    const value = typeof raw === "number" && raw !== 0 && !UNITLESS.has(prop) && !prop.startsWith("--") ? `${raw}px` : String(raw);
    return [{ prop: kebab(prop), value: normalize(value), important: false }];
  });
}

const sheetCache = new Map<string, StyleRule[]>();

function globalsRules(): StyleRule[] {
  const path = "app/globals.css";
  const cached = sheetCache.get(path);
  if (cached !== undefined) return cached;
  const rules = parseStylesheet(readFileSync(path, "utf8"));
  sheetCache.set(path, rules);
  return rules;
}

/** Declared (cascaded, not yet var-substituted) values for one element. */
export function declaredStyle(el: Element, ctx: StyleContext): Map<string, string> {
  const matched: Array<{ spec: [number, number, number]; order: number; declarations: Declaration[] }> = [];
  for (const rule of globalsRules()) {
    if (!mediaHolds(rule.media, ctx)) continue;
    const hits = rule.selectors.filter((s) => safeMatches(el, s));
    if (hits.length === 0) continue;
    const spec = hits.map(specificity).sort(compareSpecificity).at(-1) ?? [0, 0, 0];
    matched.push({ spec, order: rule.order, declarations: rule.declarations });
  }
  matched.sort((a, b) => compareSpecificity(a.spec, b.spec) || a.order - b.order);
  const out = new Map<string, string>();
  for (const m of matched) for (const d of m.declarations) if (!d.important) out.set(d.prop, d.value);
  for (const d of inlineDeclarations(el)) out.set(d.prop, d.value);
  for (const m of matched) for (const d of m.declarations) if (d.important) out.set(d.prop, d.value);
  return out;
}

function customProperty(el: Element | null, name: string, ctx: StyleContext): string | undefined {
  for (let node = el; node !== null; node = node.parentElement) {
    const v = declaredStyle(node, ctx).get(name);
    if (v !== undefined) return v;
  }
  return undefined;
}

const MAX_VAR_DEPTH = 12;

function substitute(value: string, el: Element, ctx: StyleContext, depth = 0): string {
  const at = value.indexOf("var(");
  if (at < 0 || depth > MAX_VAR_DEPTH) return value;
  let close = at + 4;
  for (let d = 1; close < value.length && d > 0; close++) {
    if (value[close] === "(") d++;
    if (value[close] === ")") d--;
  }
  const [name, ...fallback] = splitTopLevel(value.slice(at + 4, close - 1), ",");
  const found = customProperty(el, name ?? "", ctx) ?? (fallback.length > 0 ? fallback.join(", ") : "<unset>");
  const replaced = value.slice(0, at) + found + value.slice(close);
  return substitute(normalize(replaced), el, ctx, depth + 1);
}

/**
 * Every non-custom property that ends up declared on `el`, with var()
 * substituted. Compare two of these to prove two DOMs look the same.
 */
export function resolvedStyle(el: Element, ctx: StyleContext = DESKTOP): Record<string, string> {
  const out: Record<string, string> = {};
  const declared = declaredStyle(el, ctx);
  for (const prop of [...declared.keys()].sort()) {
    if (prop.startsWith("--")) continue;
    out[prop] = substitute(declared.get(prop) ?? "", el, ctx);
  }
  return out;
}

/** Marks `el` and its ancestors as hovered for the CSS side (:hover). */
export function markHovered(el: Element): void {
  for (let node: Element | null = el; node !== null; node = node.parentElement) node.setAttribute(HOVER_ATTR, "");
}

export function clearHovered(root: ParentNode = document): void {
  root.querySelectorAll(`[${HOVER_ATTR}]`).forEach((n) => n.removeAttribute(HOVER_ATTR));
  document.documentElement.removeAttribute(HOVER_ATTR);
}

/** Structural key for an element under `root` (tag names + sibling index; no classes, so renaming a class keeps the key). */
function pathOf(el: Element, root: Element): string {
  const parts: string[] = [];
  for (let node: Element | null = el; node !== null && node !== root; node = node.parentElement) {
    const index = node.parentElement === null ? 0 : [...node.parentElement.children].indexOf(node);
    parts.unshift(`${node.tagName.toLowerCase()}[${index}]`);
  }
  return [root.tagName.toLowerCase(), ...parts].join(">");
}

/** resolvedStyle of `root` and every descendant, keyed by structural path. */
export function styleTree(root: Element, ctx: StyleContext = DESKTOP): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = { [pathOf(root, root)]: resolvedStyle(root, ctx) };
  root.querySelectorAll("*").forEach((el) => {
    // An icon's paths carry no styles of their own; the <svg> itself is enough.
    if (el.parentElement?.closest("svg") != null) return;
    out[pathOf(el, root)] = resolvedStyle(el, ctx);
  });
  return out;
}

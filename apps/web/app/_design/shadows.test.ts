import { readFileSync, globSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { DESKTOP, declaredStyle, splitTopLevel } from "./testing/declaredStyle";

/*
 * Shadow inventory, enforced (PLAN-redesign-home F2a, review E10; the table is
 * .claude/docs/design/reviews/shadow-inventory.md). Shadows come in three
 * families:
 *   (a) the halo under an icon square     → var(--shadow-icon)
 *   (b) elevation: FAB, popover, primary  → var(--shadow-*) tokens
 *   (c) inset stripes and status rings    → literal, on purpose: they are
 *       semantic marks (a row's status, a dot's ring, an invalid field), not
 *       depth, and a look layer must never flatten them.
 * A shadow with px outside (c) means a new depth someone typed by hand, which
 * the look layer (F2b) cannot reach. Put it in a token instead.
 */

/** Family (c), and nothing else: every layer is an inset stripe or a zero-offset ring. */
function isMarkNotDepth(shadow: string): boolean {
  return splitTopLevel(shadow, ",").every((layer) => /^inset\s+\d/.test(layer) || /^0 0 0 \d+(\.\d+)?px\s/.test(layer));
}

/** The JS expression after `boxShadow:`, up to its top-level `,` or `}`. */
function expressionAt(src: string, from: number): string {
  let depth = 0;
  let quote: string | null = null;
  let i = from;
  for (; i < src.length; i++) {
    const ch = src[i] ?? "";
    if (quote !== null) {
      if (ch === quote) quote = null;
      continue;
    }
    if (ch === "\"" || ch === "'" || ch === "`") quote = ch;
    else if ("([{".includes(ch)) depth++;
    else if (")]}".includes(ch)) {
      if (depth === 0) break;
      depth--;
    } else if (ch === "," && depth === 0) break;
  }
  return src.slice(from, i);
}

/** Each string literal in the expression; a template's ${…} counts as a word. */
function literals(expression: string): string[] {
  return [...expression.matchAll(/"([^"]*)"|'([^']*)'|`([^`]*)`/g)].map((m) =>
    (m[1] ?? m[2] ?? m[3] ?? "").replace(/\$\{[^}]*\}/g, "X").replace(/\s+/g, " ").trim()
  );
}

const lineOf = (src: string, index: number): number => src.slice(0, index).split("\n").length;

function offendersInSource(): string[] {
  return globSync("app/**/*.{ts,tsx}")
    .filter((f) => !f.includes(".test.") && !f.includes("/testing/"))
    .sort()
    .flatMap((f) => {
      const src = readFileSync(f, "utf8");
      return [...src.matchAll(/boxShadow:\s*/g)].flatMap((m) =>
        literals(expressionAt(src, (m.index ?? 0) + m[0].length))
          .filter((s) => /\d(\.\d+)?px/.test(s) && !isMarkNotDepth(s))
          .map((s) => `${f}:${lineOf(src, m.index ?? 0)} ${s}`)
      );
    });
}

function offendersInStylesheet(): string[] {
  const css = readFileSync("app/globals.css", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
  // Token DEFINITIONS (--shadow-*: …) are where literal shadows belong; a
  // box-shadow declaration must use one, or be a family (c) mark.
  return [...css.matchAll(/(?<![-\w])box-shadow:\s*([^;}]+)/g)]
    .map((m) => (m[1] ?? "").trim())
    .filter((v) => /\d(\.\d+)?px/.test(v) && !isMarkNotDepth(v))
    .map((v) => `globals.css box-shadow: ${v}`);
}

describe("shadow inventory", () => {
  it("family (c) is recognised narrowly", () => {
    expect(isMarkNotDepth("inset 3px 0 0 X")).toBe(true);
    expect(isMarkNotDepth("0 0 0 3px color-mix(in oklab, X 22%, transparent)")).toBe(true);
    expect(isMarkNotDepth("0 0 0 3px var(--surface), 0 0 0 5px var(--lime)")).toBe(true);
    expect(isMarkNotDepth("0 6px 16px X")).toBe(false);
    expect(isMarkNotDepth("0 0 0 3px X, 0 4px 14px Y")).toBe(false);
  });

  it("no component types a px shadow outside family (c)", () => {
    expect(offendersInSource()).toEqual([]);
  });

  it("globals.css declares no px shadow outside a token or family (c)", () => {
    expect(offendersInStylesheet()).toEqual([]);
  });

  // F2a moved family (b) into tokens with "no visual change": each token must
  // hold, character for character, what its components typed inline before.
  it("the elevation tokens keep the values the components typed inline", () => {
    const root = declaredStyle(document.documentElement, DESKTOP);
    expect({
      fab: root.get("--shadow-fab"),
      primary: root.get("--shadow-primary"),
      primarySm: root.get("--shadow-primary-sm"),
      tip: root.get("--shadow-tip"),
      logoBadge: root.get("--shadow-logo-badge")
    }).toEqual({
      fab: "0 10px 30px color-mix(in oklab, var(--lime) 45%, transparent)",
      primary: "0 4px 14px color-mix(in oklab, var(--lime) 40%, transparent)",
      primarySm: "0 3px 10px color-mix(in oklab, var(--lime) 36%, transparent)",
      tip: "0 8px 24px rgba(0,0,0,.18)",
      logoBadge: "0 6px 20px rgba(0,0,0,.18)"
    });
  });
});

import { readFileSync } from "node:fs";
import { render } from "@testing-library/react";
import type { CSSProperties, ReactElement } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { ColorKey } from "./colors";
import { ModuleHeaderIcon } from "./ModuleHeaderIcon";
import { contrastRatio, mixOklab, parseHex } from "./testing/contrast";
import {
  DESKTOP,
  clearHovered,
  declaredStyle,
  markFocusVisible,
  markHovered,
  resolvedStyle,
  type StyleContext
} from "./testing/declaredStyle";

/*
 * F2b (PLAN-redesign-home §3.3): the "Captions adaptado" look, a layer of
 * tokens and rules under html[data-look="refresh"], in light and dark. These
 * tests switch the attribute on by hand and read the cascade of globals.css
 * (testing/declaredStyle.ts). That the layer changes nothing while the
 * attribute is absent is look.off.characterization.test.tsx's job.
 */

const TALL: StyleContext = { width: 1280, height: 800 };
const SHORT: StyleContext = { width: 1280, height: 480 };
const PRINT: StyleContext = { width: 1280, print: true };
const FORCED: StyleContext = { width: 1280, forcedColors: true };
const THEMES = ["light", "dark"] as const;
type Theme = (typeof THEMES)[number];

const html = (): HTMLElement => document.documentElement;

function setLook(theme: Theme): void {
  html().setAttribute("data-theme", theme);
  html().setAttribute("data-look", "refresh");
}

afterEach(() => {
  clearHovered();
  html().removeAttribute("data-theme");
  html().removeAttribute("data-look");
  document.body.innerHTML = "";
});

/** A token as declared on <html>, var() left as written. */
const token = (name: string, ctx: StyleContext = DESKTOP): string => declaredStyle(html(), ctx).get(name) ?? "<unset>";

/** A token with its var() substituted, read through a probe that uses it. */
function resolvedToken(name: string, ctx: StyleContext = DESKTOP): string {
  const probe = render(<div style={{ outlineColor: `var(${name})` }} />).container.firstElementChild;
  if (probe === null) throw new Error("no probe");
  return resolvedStyle(probe, ctx)["outline-color"] ?? "<unset>";
}

type Vars = CSSProperties & Record<`--${string}`, string>;

function only(ui: ReactElement): HTMLElement {
  const el = render(ui).container.firstElementChild;
  if (!(el instanceof HTMLElement)) throw new Error("nothing rendered");
  return el;
}

const HAIR = { light: "#E2E9DF", dark: "#1F3828" } as const;

describe("look refresh: tokens", () => {
  for (const theme of THEMES) {
    it(`cards lose their shadow, take a hairline and a 16px radius (${theme})`, () => {
      setLook(theme);
      expect(token("--shadow-card")).toBe("none");
      expect(token("--hair")).toBe(HAIR[theme]);
      expect(token("--r-lg")).toBe("16px");
    });

    // Review D4: the halo is lime, offset 14px down, and NEVER a 0-offset glow
    // (on #07150D that is the dark-glow pattern); in dark it drops to .10.
    it(`the hover halo is lime, 14px down, never a zero-offset glow (${theme})`, () => {
      setLook(theme);
      const halo = resolvedToken("--halo");
      expect(halo).toMatch(/^0 14px /);
      expect(halo).toMatch(theme === "dark" ? /rgba\(122,\s*196,\s*0,\s*\.10\)/ : /#7AC400/);
    });

    it(`the page is paper with grain and a green wash at the top (${theme})`, () => {
      setLook(theme);
      const grad = resolvedToken("--bg-grad");
      expect(grad).toContain("data:image/svg+xml");
      expect(grad).toContain("feTurbulence");
      const opacity = Number(/opacity='([\d.]+)'/.exec(grad)?.[1]);
      expect(opacity).toBeGreaterThanOrEqual(0.05);
      expect(opacity).toBeLessThanOrEqual(0.07);
      expect(grad).toMatch(/radial-gradient\([^)]*at 50% -/);
    });
  }

  it("the light page is paper white", () => {
    setLook("light");
    expect(token("--bg")).toBe("#FBFBF8");
  });

  it("the grain never pins itself to the viewport (no background-attachment: fixed)", () => {
    const css = readFileSync("app/globals.css", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    expect(css).not.toMatch(/background-attachment:\s*fixed/);
    expect(css).not.toMatch(/background:[^;]*\bfixed\b/);
  });

  it("is a screen look: printed pages (receipts) keep today's tokens", () => {
    setLook("light");
    expect(token("--r-lg", PRINT)).toBe("22px");
    expect(token("--shadow-card", PRINT)).not.toBe("none");
    expect(token("--hair", PRINT)).toBe("<unset>");
  });
});

describe("look refresh: cards", () => {
  for (const theme of THEMES) {
    it(`a module card rests flat on a hairline and hovers with the halo and its own color (${theme})`, () => {
      setLook(theme);
      const card = only(<button className="module-card" style={{ "--mod-c": "var(--purple)" } as Vars} />);
      const rest = resolvedStyle(card);
      expect(rest["border"]).toBe(`1px solid ${HAIR[theme]}`);
      expect(rest["box-shadow"]).toBe("none");
      expect(rest["border-radius"]).toBe("16px");

      markHovered(card);
      const hover = resolvedStyle(card);
      expect(hover["box-shadow"]).toBe(resolvedToken("--halo"));
      // Lime rule (review D4): the border takes the card's color, not lime.
      expect(hover["border"]).toBe(`1px solid color-mix(in oklab, #8E2E8E 45%, ${HAIR[theme]})`);
    });

    it(`a KPI card does the same with its own color (${theme})`, () => {
      setLook(theme);
      const card = only(<button className="kpi-card" style={{ "--kpi-c": "var(--orange)" } as Vars} />);
      expect(resolvedStyle(card)["border"]).toBe(`1px solid ${HAIR[theme]}`);
      expect(resolvedStyle(card)["box-shadow"]).toBe("none");
      markHovered(card);
      expect(resolvedStyle(card)["box-shadow"]).toBe(resolvedToken("--halo"));
      expect(resolvedStyle(card)["border"]).toBe(`1px solid color-mix(in oklab, #F39200 45%, ${HAIR[theme]})`);
    });
  }

  it("forced colors: the cards keep a CanvasText border (the halo and hairlines vanish there)", () => {
    setLook("light");
    for (const cls of ["module-card", "kpi-card"]) {
      const card = only(<button className={cls} style={{ "--mod-c": "var(--red)", "--kpi-c": "var(--red)" } as Vars} />);
      expect(resolvedStyle(card, FORCED)["border"]).toBe("1px solid CanvasText");
      markHovered(card);
      expect(resolvedStyle(card, FORCED)["border"]).toBe("1px solid CanvasText");
      clearHovered();
    }
  });
});

describe("look refresh: tinted icon squares", () => {
  const COLORED: ColorKey[] = ["lime", "forest", "yellow", "orange", "purple", "red"];

  for (const theme of THEMES) {
    it(`header squares are tinted at 13% with the glyph in the color, no halo (${theme})`, () => {
      setLook(theme);
      const square = only(<ModuleHeaderIcon icon="students" color="forest" />);
      const style = resolvedStyle(square);
      expect(style["background"]).toMatch(/^color-mix\(in oklab, #0A7A2F 13%, #[0-9A-F]{6}\)$/i);
      expect(style["color"]).toMatch(/^color-mix\(in oklab, #0A7A2F \d+%, #[0-9A-F]{6}\)$/i);
      expect(style["box-shadow"]).toBe("none");
    });

    it(`the home's module and KPI icons are tinted the same way (${theme})`, () => {
      setLook(theme);
      const card = only(
        <button className="module-card" style={{ "--mod-c": "var(--forest)" } as Vars}>
          <div className="module-card-icon" />
        </button>
      );
      const icon = card.firstElementChild as Element;
      expect(resolvedStyle(icon)["background"]).toMatch(/^color-mix\(in oklab, #0A7A2F 13%,/);
      expect(resolvedStyle(icon)["box-shadow"]).toBe("none");
      const kpi = only(
        <button className="kpi-card" style={{ "--kpi-c": "var(--forest)" } as Vars}>
          <div className="kpi-card-icon" />
        </button>
      );
      expect(resolvedStyle(kpi.firstElementChild as Element)["background"]).toMatch(/^color-mix\(in oklab, #0A7A2F 13%,/);
    });

    it(`the neutral square stays grey (${theme})`, () => {
      setLook(theme);
      const style = resolvedStyle(only(<ModuleHeaderIcon icon="students" color="neutral" />));
      expect(style["background"]).not.toMatch(/color-mix/);
      expect(style["box-shadow"]).toBe("none");
    });

    // WCAG 1.4.11: the glyph is the only thing that says what the square is.
    // Pure lime/yellow/orange on their own tint, or forest/purple on dark,
    // fall under 3:1; the glyph is pulled toward --text until it holds.
    it(`every colored glyph holds 3:1 on its tint (${theme})`, () => {
      setLook(theme);
      for (const color of COLORED) {
        const style = resolvedStyle(only(<ModuleHeaderIcon icon="students" color={color} />));
        const bg = /^color-mix\(in oklab, (#\w{6}) (\d+)%, (#\w{6})\)$/i.exec(style["background"] ?? "");
        const fg = /^color-mix\(in oklab, (#\w{6}) (\d+)%, (#\w{6})\)$/i.exec(style["color"] ?? "");
        if (bg === null || fg === null) throw new Error(`${color}: not a tinted square`);
        const paint = (m: RegExpExecArray) => mixOklab(parseHex(m[1] ?? ""), Number(m[2]) / 100, parseHex(m[3] ?? ""));
        expect.soft(contrastRatio(paint(fg), paint(bg)), `${theme} ${color}`).toBeGreaterThanOrEqual(3);
      }
    });
  }
});

describe("look refresh: the floating top bar", () => {
  for (const theme of THEMES) {
    it(`is a frosted capsule 12px from the edges (${theme})`, () => {
      setLook(theme);
      const bar = only(<header className="topbar" />);
      const style = resolvedStyle(bar, TALL);
      expect(style["margin"]).toBe("12px 12px 0");
      expect(style["border-radius"]).toBe("28px");
      expect(style["border"]).toBe(`1px solid ${HAIR[theme]}`);
      expect(style["backdrop-filter"]).toMatch(/^blur\(28px\) saturate\(\d+%\)$/);
      expect(style["-webkit-backdrop-filter"]).toBe(style["backdrop-filter"]);
    });
  }

  it("sticks (12px down) only when the window is at least 560px tall", () => {
    setLook("light");
    const bar = only(<header className="topbar" />);
    expect(resolvedStyle(bar, TALL)).toMatchObject({ position: "sticky", top: "12px" });
    expect(resolvedStyle(bar, SHORT)["position"]).toBe("relative");
  });

  // WCAG 2.2 · 2.4.11: the sticky bar must never cover the focused field.
  // It counts the 12px the capsule floats down too: 78 + 16 alone left a 4px
  // gap, and a 2px ring at 2px offset touched the bar (home.responsive.spec.ts).
  it("scroll padding keeps a focused field clear of the sticky bar", () => {
    setLook("light");
    expect(resolvedStyle(html(), TALL)["scroll-padding-top"]).toBe("calc(calc(78px + 12px) + 16px)");
    expect(resolvedStyle(html(), SHORT)["scroll-padding-top"]).toBeUndefined();
  });

  it("without backdrop-filter support the bar goes opaque", () => {
    const css = readFileSync("app/globals.css", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    expect(css).toMatch(
      /@supports not \(\(backdrop-filter: blur\(1px\)\) or \(-webkit-backdrop-filter: blur\(1px\)\)\)\s*\{\s*html\[data-look="refresh"\] \.topbar\s*\{\s*background: var\(--surface\);/
    );
  });

  // The "Paso N de 8" bar (Configuración, < 900px) sticks right under the
  // floating bar, and the scroll padding leaves room for both.
  it("the setup step bar sticks under the capsule, not under it", () => {
    setLook("light");
    const step = only(<div className="setup-mobilebar" />);
    const narrowTall: StyleContext = { width: 800, height: 800 };
    expect(resolvedStyle(step, narrowTall)["top"]).toBe("calc(78px + 12px)");
    expect(resolvedStyle(step, { width: 800, height: 480 })["top"]).toBe("0px");
    expect(resolvedStyle(html(), narrowTall)["scroll-padding-top"]).toBe("calc(calc(78px + 12px) + 48px + 12px)");
  });
});

describe("look refresh: focus ring", () => {
  // Review D10: today's ring is lime at 55% (~1.6:1 on white). With flat cards
  // the ring is all that is left, so it must hold 3:1 against both the card
  // surface and the page.
  for (const theme of THEMES) {
    it(`is 2px solid, 2px off, and holds 3:1 on surface and page (${theme})`, () => {
      setLook(theme);
      const button = only(<button />);
      markFocusVisible(button);
      const style = resolvedStyle(button);
      const ring = /^2px solid (#\w{6})$/.exec(style["outline"] ?? "");
      expect(ring?.[1]).toBe(theme === "dark" ? "#7AC400" : "#0A7A2F");
      expect(style["outline-offset"]).toBe("2px");
      const color = parseHex(ring?.[1] ?? "#000000");
      expect(contrastRatio(color, parseHex(resolvedToken("--surface")))).toBeGreaterThanOrEqual(3);
      expect(contrastRatio(color, parseHex(resolvedToken("--bg")))).toBeGreaterThanOrEqual(3);
    });
  }
});

describe("look refresh: the greeting serif", () => {
  it("exposes .look-greeting: Libre Baskerville 400, only under the look", () => {
    const h1 = only(<h1 className="look-greeting" />);
    expect(resolvedStyle(h1)["font-weight"]).toBeUndefined();
    setLook("light");
    // next/font sets --font-libre-baskerville on <html> by class (layout.tsx).
    expect(token("--font-serif")).toMatch(/^var\(--font-libre-baskerville\), Georgia, /);
    expect(declaredStyle(h1, DESKTOP).get("font-family")).toBe("var(--font-serif)");
    expect(resolvedStyle(h1)["font-weight"]).toBe("400");
  });
});

describe("look refresh: OFF stays OFF", () => {
  it("every look rule is scoped under html[data-look=\"refresh\"]", () => {
    const css = readFileSync("app/globals.css", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    const mentions = [...css.matchAll(/data-look(?!="refresh"\])/g)];
    expect(mentions).toEqual([]);
  });

  it("a colored square keeps today's solid fill without the attribute", () => {
    html().setAttribute("data-theme", "light");
    const style = resolvedStyle(only(<ModuleHeaderIcon icon="students" color="forest" />));
    expect(style["background"]).toBe("#0A7A2F");
  });
});

import { readFileSync } from "node:fs";

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { SetupStepList } from "./SetupStepList";
import { StepFrame } from "./StepFrame";
import { PENDING_TEXT, QUIET_TEXT } from "./setupColors";
import { stepView, stepViews } from "./stepFixtures";

/*
 * The setup view's small text (13 px: the pending line, "PASO N DE 8", the
 * Herramientas helper) must reach WCAG AA 4.5:1 in both themes. jsdom paints
 * nothing, so this resolves the colours from the tokens in globals.css and
 * measures them against the surfaces they sit on.
 */

const css = readFileSync("app/globals.css", "utf8");

type Rgb = readonly [number, number, number];

function themeTokens(selector: string): Record<string, string> {
  const start = css.indexOf(`${selector} {`);
  const body = css.slice(start, css.indexOf("}", start));
  return Object.fromEntries([...body.matchAll(/(--[\w-]+):\s*(#[0-9A-Fa-f]{6})/g)].map((m) => [m[1], m[2]]));
}

const LIGHT = { ...themeTokens(":root"), ...themeTokens(':root, [data-theme="light"]') };
const DARK = { ...LIGHT, ...themeTokens('[data-theme="dark"]') };

const hex = (value: string): Rgb => [1, 3, 5].map((i) => parseInt(value.slice(i, i + 2), 16) / 255) as unknown as Rgb;
const toLinear = (c: number): number => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
const fromLinear = (c: number): number => (c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055);
const clamp = (c: number): number => Math.min(1, Math.max(0, c));

function toOklab([r, g, b]: Rgb): Rgb {
  const [lr, lg, lb] = [r, g, b].map(toLinear) as unknown as Rgb;
  const l = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);
  return [0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s, 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s, 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s];
}

function fromOklab([L, a, b]: Rgb): Rgb {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const lin = [4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s, -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s, -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s];
  return lin.map((c) => clamp(fromLinear(c))) as unknown as Rgb;
}

/** Resolves `var(--x)` and `color-mix(in oklab, var(--x) N%, var(--y))` against a theme. */
function resolve(value: string, theme: Record<string, string>): Rgb {
  const token = (name: string): Rgb => {
    const raw = theme[name];
    if (raw === undefined) throw new Error(`no token ${name}`);
    return hex(raw);
  };
  const mix = /^color-mix\(in oklab, var\((--[\w-]+)\) (\d+)%, var\((--[\w-]+)\)\)$/.exec(value);
  if (mix !== null) {
    const p = Number(mix[2]) / 100;
    const [x, y] = [toOklab(token(mix[1]!)), toOklab(token(mix[3]!))];
    return fromOklab([0, 1, 2].map((i) => x[i]! * p + y[i]! * (1 - p)) as unknown as Rgb);
  }
  const plain = /^var\((--[\w-]+)\)$/.exec(value);
  if (plain === null) throw new Error(`cannot resolve ${value}`);
  return token(plain[1]!);
}

const luminance = (rgb: Rgb): number => {
  const [r, g, b] = rgb.map(toLinear) as unknown as Rgb;
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};

function contrast(a: Rgb, b: Rgb): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
  return (hi + 0.05) / (lo + 0.05);
}

/** WCAG AA for text under 18.66 px bold / 24 px regular. */
const AA_SMALL_TEXT = 4.5;

describe("setup small text contrast (design review N7)", () => {
  const themes = [["light", LIGHT], ["dark", DARK]] as const;

  it.each(themes)("the pending line reads at 4.5:1 on the card and on the open row (%s)", (_name, theme) => {
    const text = resolve(PENDING_TEXT, theme);
    expect(contrast(text, resolve("var(--surface)", theme))).toBeGreaterThanOrEqual(AA_SMALL_TEXT);
    const openRow = fromOklab((() => {
      const [lime, surface] = [toOklab(resolve("var(--lime)", theme)), toOklab(resolve("var(--surface)", theme))];
      return [0, 1, 2].map((i) => lime[i]! * 0.18 + surface[i]! * 0.82) as unknown as Rgb;
    })());
    expect(contrast(text, openRow)).toBeGreaterThanOrEqual(AA_SMALL_TEXT);
  });

  it.each(themes)("the quiet text (eyebrow, helper) reads at 4.5:1 on the card (%s)", (_name, theme) => {
    expect(contrast(resolve(QUIET_TEXT, theme), resolve("var(--surface)", theme))).toBeGreaterThanOrEqual(AA_SMALL_TEXT);
  });
});

describe("setup small text uses those colours", () => {
  it("the pending line and the Herramientas helper", () => {
    render(<SetupStepList steps={stepViews()} cardTitle="Puesta en marcha" openId="groups" onOpen={vi.fn()} onOpenStudentImport={vi.fn()} showTrash={false} narrow={false} />);
    expect(screen.getByText("Falta Grupos")).toHaveStyle({ color: PENDING_TEXT });
    expect(screen.getByText("Necesita niveles, grados y grupos.")).toHaveStyle({ color: QUIET_TEXT });
  });

  it("the 'Paso N de 8' eyebrow", () => {
    render(<StepFrame step={stepView("levels")} onGo={() => {}}>x</StepFrame>);
    expect(screen.getByText("Paso 3 de 8")).toHaveStyle({ color: QUIET_TEXT });
  });
});

/*
 * WCAG 2.x contrast for tests, plus CSS `color-mix(in oklab, …)` so a token
 * built from a mix can be measured the way the browser paints it. Hex colors
 * only (#RRGGBB): every token the look layer measures is one.
 */

type Rgb = readonly [number, number, number];

const HEX = /^#([0-9a-f]{6})$/i;

export function parseHex(hex: string): Rgb {
  const m = HEX.exec(hex.trim());
  if (m === null) throw new Error(`not a #RRGGBB color: ${hex}`);
  const digits = m[1] ?? "";
  return [0, 2, 4].map((i) => parseInt(digits.slice(i, i + 2), 16) / 255) as unknown as Rgb;
}

const toLinear = (c: number): number => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
const toGamma = (c: number): number => (c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055);
const clamp01 = (c: number): number => Math.min(1, Math.max(0, c));

function toOklab(rgb: Rgb): Rgb {
  const [r, g, b] = rgb.map(toLinear) as unknown as Rgb;
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s
  ];
}

function fromOklab([L, a, b]: Rgb): Rgb {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s
  ].map((c) => clamp01(toGamma(c))) as unknown as Rgb;
}

/** `color-mix(in oklab, a <share>, b)`, share in 0..1. */
export function mixOklab(a: Rgb, share: number, b: Rgb): Rgb {
  const x = toOklab(a);
  const y = toOklab(b);
  return fromOklab(x.map((v, i) => v * share + (y[i] ?? 0) * (1 - share)) as unknown as Rgb);
}

function luminance(rgb: Rgb): number {
  const [r, g, b] = rgb.map(toLinear) as unknown as Rgb;
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

const LUMINANCE_FLARE = 0.05;

export function contrastRatio(a: Rgb, b: Rgb): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
  return (hi + LUMINANCE_FLARE) / (lo + LUMINANCE_FLARE);
}

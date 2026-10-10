import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

/*
 * The setup view animates exactly two moments (PLAN §1.7). jsdom runs no CSS,
 * so this reads the rules themselves: what they animate, for how long, and that
 * every transform ENDS at `none` — a lingering transform would make the open
 * step the containing block of anything `position: fixed` inside it.
 */

const css = readFileSync("app/globals.css", "utf8");

function block(selector: RegExp): string {
  const match = new RegExp(`${selector.source}\\s*\\{([^}]*)\\}`).exec(css);
  return match?.[1] ?? "";
}

function keyframes(name: string): string {
  const match = new RegExp(`@keyframes ${name}\\s*\\{([\\s\\S]*?\\})\\s*\\}`).exec(css);
  return match?.[1] ?? "";
}

describe("setup view motion (§1.7)", () => {
  it("the open step's body fades in from 8 px below in 160 ms ease-out, ending at transform: none", () => {
    const rule = block(/\.setup-step-body/);
    expect(rule).toMatch(/animation:\s*setupStepIn\s+160ms\s+ease-out/);
    const frames = keyframes("setupStepIn");
    expect(frames).toMatch(/from\s*\{[^}]*opacity:\s*0[^}]*translateY\(8px\)/);
    expect(frames).toMatch(/to\s*\{[^}]*transform:\s*none/);
  });

  it("a step turning done scales its check in from 0.8 in 180 ms, ending at transform: none", () => {
    expect(block(/\.setup-check-in/)).toMatch(/animation:\s*setupCheckIn\s+180ms/);
    const frames = keyframes("setupCheckIn");
    expect(frames).toMatch(/from\s*\{[^}]*scale\(0?\.8\)/);
    expect(frames).toMatch(/to\s*\{[^}]*transform:\s*none/);
  });

  it("reduced motion drops the shift and the scale", () => {
    const reduced = /@media \(prefers-reduced-motion: reduce\)\s*\{([^@]*\.setup-step-body[^@]*)\}/.exec(css)?.[1] ?? "";
    expect(reduced).toMatch(/\.setup-check-in/);
    expect(reduced).toMatch(/animation:\s*none|animation-name:\s*setupFade/);
  });
});

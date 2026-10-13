import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

/*
 * The Avisos view's layout and motion live in globals.css (`.announce-*`,
 * PLAN-redesign-avisos §10, D5, D6, D15). jsdom runs no CSS, so this reads the
 * rules themselves.
 */

const css = readFileSync("app/globals.css", "utf8");

/** Every `.announce…` rule body, plus the at-rules that wrap them. */
const announce = css.slice(css.indexOf("Avisos · \"Destinatarios al lado\""), css.indexOf("LOOK REFRESH"));

function block(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`(?:^|\\n)\\s*${escaped}\\s*\\{([^}]*)\\}`).exec(announce)?.[1] ?? "";
}

function keyframes(name: string): string {
  return new RegExp(`@keyframes ${name}\\s*\\{([\\s\\S]*?\\})\\s*\\}`).exec(announce)?.[1] ?? "";
}

describe("Avisos view styles", () => {
  it("lays the form out as Destinatarios at 300 px beside the message", () => {
    const grid = block(".announce-grid");
    expect(grid).toMatch(/grid-template-columns:\s*300px minmax\(0, 1fr\)/);
    expect(grid).toMatch(/grid-template-areas:\s*"pane editor" "pane footer"/);
  });

  it("goes to one column below 860 px of its own column, with the send footer stuck to the bottom", () => {
    expect(block(".announce")).toMatch(/container-type:\s*inline-size/);
    const narrow = /@container \(max-width: 859px\)\s*\{([\s\S]*?)\n\}/.exec(announce)?.[1] ?? "";
    expect(narrow).toMatch(/grid-template-areas:\s*"pane" "editor" "footer"/);
    expect(narrow).toMatch(/\.announce-footer\s*\{[^}]*position:\s*sticky;[^}]*bottom:\s*0/);
  });

  // WCAG 2.2 · 2.4.11 (D6)
  it("keeps a focused field clear of the stuck footer", () => {
    expect(announce).toMatch(/html:has\(\.announce-footer\)\s*\{\s*scroll-padding-bottom:\s*var\(--announce-footer-h\)/);
  });

  it("animates only the check and the folded panel opening, ending at transform: none", () => {
    expect(block(".announce-mark__check")).toMatch(/animation:\s*announceCheckIn\s+160ms/);
    expect(block(".announce-auto[open] .announce-auto__body")).toMatch(/animation:\s*announceOpen\s+180ms/);
    for (const name of ["announceCheckIn", "announceOpen"]) {
      expect(keyframes(name)).toMatch(/to\s*\{\s*transform:\s*none/);
      expect(keyframes(name)).not.toMatch(/opacity/);
    }
  });

  it("never transitions everything", () => {
    expect(announce).not.toMatch(/transition:\s*all/);
  });

  it("stills it all under reduced motion", () => {
    const reduced = /@media \(prefers-reduced-motion: reduce\)\s*\{([\s\S]*?)\n\}/.exec(announce)?.[1] ?? "";
    expect(reduced).toMatch(/\.announce-mark__check/);
    expect(reduced).toMatch(/\.announce-auto\[open\] \.announce-auto__body\s*\{\s*animation:\s*none/);
    expect(reduced).toMatch(/\.announce-auto__chevron\s*\{\s*transition:\s*none/);
  });

  // D15: cards sit on the refresh look's hairline when it is on.
  it("draws its cards on the hairline token, falling back to the border", () => {
    expect(block(".announce-card")).toMatch(/border:\s*1px solid var\(--hair, var\(--border\)\)/);
  });
});

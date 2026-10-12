import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

/*
 * The look refresh's greeting serif (PLAN-redesign-home §3.3, F2b):
 * `--font-serif` in globals.css reads `--font-libre-baskerville`, which only
 * exists if the root layout loads the font through next/font and puts its
 * variable class on <html>. RootLayout is an async server component reading
 * cookies, so this guards the wiring at the source.
 */

const layout = readFileSync("app/layout.tsx", "utf8");

describe("root layout fonts", () => {
  it("loads Libre Baskerville 400 through next/font as --font-libre-baskerville", () => {
    expect(layout).toMatch(/import \{[^}]*\bLibre_Baskerville\b[^}]*\} from "next\/font\/google"/);
    const call = /Libre_Baskerville\(\{([\s\S]*?)\}\)/.exec(layout)?.[1] ?? "";
    expect(call).toMatch(/weight:\s*\["400"\]/);
    expect(call).toMatch(/variable:\s*"--font-libre-baskerville"/);
  });

  // Only the greeting under module_ui_refresh uses it: with the look OFF, a
  // preload would make every user download a font nothing paints.
  it("does not preload it", () => {
    const call = /Libre_Baskerville\(\{([\s\S]*?)\}\)/.exec(layout)?.[1] ?? "";
    expect(call).toMatch(/preload:\s*false/);
  });

  it("puts its variable class on <html>", () => {
    expect(layout).toMatch(/className=\{`[^`]*\$\{libreBaskerville\.variable\}[^`]*`\}/);
  });
});

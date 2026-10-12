import { render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Providers } from "../test-utils";
import { USERS } from "./data";
import { DESKTOP, PHONE, styleTree } from "./testing/declaredStyle";
import { TopBar } from "./TopBar";

/*
 * F2a characterization (PLAN-redesign-home §3.3, review E6/D1): the top bar's
 * styles move from inline to `.topbar` in globals.css with ZERO visual change.
 * The snapshot was recorded against the inline version; every element of the
 * bar must resolve to the same declared values afterwards, on a desk and on a
 * phone (where the 768px block applies), in both themes.
 * See testing/declaredStyle.ts for why this is not jsdom's getComputedStyle.
 */

function renderBar(): HTMLElement {
  const { container } = render(
    <Providers>
      <TopBar user={USERS.admin} theme="light" onToggle={() => {}} onHome={() => {}} onLogout={() => {}} onOpenPalette={() => {}} />
    </Providers>
  );
  const header = container.querySelector("header");
  if (header === null) throw new Error("TopBar rendered no <header>");
  return header;
}

afterEach(() => {
  document.documentElement.removeAttribute("data-theme");
});

describe("TopBar styles (characterization)", () => {
  for (const theme of ["light", "dark"] as const) {
    it(`resolves to today's values on a desk (${theme})`, () => {
      const header = renderBar();
      document.documentElement.setAttribute("data-theme", theme);
      expect(styleTree(header, DESKTOP)).toMatchSnapshot();
    });
  }

  it("resolves to today's values on a phone (the 768px block)", () => {
    const header = renderBar();
    document.documentElement.setAttribute("data-theme", "light");
    expect(styleTree(header, PHONE)).toMatchSnapshot();
  });
});

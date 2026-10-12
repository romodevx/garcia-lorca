import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Providers } from "../test-utils";
import { USERS } from "./data";
import { resolvedStyle } from "./testing/declaredStyle";
import { TopBar } from "./TopBar";

describe("TopBar", () => {
  // An inline style beats every author rule, so a look layered under
  // html[data-look] (F2b) could not restyle the bar while its own elements
  // carried one. Children rendered by other components (Logo, Avatar, IconBtn)
  // are their components' business.
  it("its own elements carry no inline style, so the stylesheet owns the look", () => {
    const { container } = render(
      <Providers>
        <TopBar user={USERS.admin} theme="light" onToggle={() => {}} onHome={() => {}} onLogout={() => {}} onOpenPalette={() => {}} />
      </Providers>
    );
    const own = container.querySelectorAll(
      "header, .topbar-logo, .topbar-search, .topbar-search > button, .topbar-search-text, .topbar-search-kbd, .topbar-spacer, .topbar-actions, .topbar-user, .topbar-user-text, .topbar-user-text > div"
    );
    expect(own.length).toBe(12);
    const styled = [...own].filter((el) => el.hasAttribute("style")).map((el) => el.className || el.tagName);
    expect(styled).toEqual([]);
  });

  // PLAN-redesign-home §4.2: below 768px the refreshed look drops the theme
  // toggle, as it already drops the language one; it needs a box to aim at.
  it("wraps the theme toggle so the stylesheet can drop it on a narrow window", () => {
    const { container } = render(
      <Providers>
        <TopBar user={USERS.admin} theme="light" onToggle={() => {}} onHome={() => {}} onLogout={() => {}} onOpenPalette={() => {}} />
      </Providers>
    );
    const wrapper = container.querySelector(".topbar-actions > .topbar-theme");
    expect(wrapper?.querySelector("button[aria-label='Cambiar tema']")).not.toBeNull();
    expect(wrapper?.hasAttribute("style")).toBe(false);
  });

  // QA 2026-10-12: at 820px the search box is ~170px wide and the placeholder
  // wrapped onto five lines spilling out of the 50px pill (three with the
  // classic look). It must stay one line and end in an ellipsis, in both looks.
  it.each([
    ["classic", null],
    ["refresh", "refresh"],
  ])("keeps the search placeholder on one line (%s look)", (_name, look) => {
    if (look) document.documentElement.setAttribute("data-look", look);
    try {
      const { container } = render(
        <Providers>
          <TopBar user={USERS.admin} theme="light" onToggle={() => {}} onHome={() => {}} onLogout={() => {}} onOpenPalette={() => {}} />
        </Providers>
      );
      const text = container.querySelector(".topbar-search-text");
      expect(text).not.toBeNull();
      const style = resolvedStyle(text!, { width: 820 });
      expect(style).toMatchObject({ "white-space": "nowrap", overflow: "hidden", "text-overflow": "ellipsis", "min-width": "0" });
    } finally {
      document.documentElement.removeAttribute("data-look");
    }
  });
});

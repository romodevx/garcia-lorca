import { render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ModuleHeaderIcon } from "./ModuleHeaderIcon";
import { DESKTOP, PHONE, declaredStyle, markFocusVisible, resolvedStyle, type StyleContext } from "./testing/declaredStyle";

/*
 * F2b's promise (PLAN-redesign-home §1): with module_ui_refresh OFF the app is
 * IDENTICAL to today. F2a's snapshots cover the top bar and the home cards;
 * this one covers everything else the look layer touches — every token on
 * <html>, the page background, the header icon squares, the focus ring —
 * recorded against the sheet BEFORE the layer existed. No data-look here.
 */

const SHORT: StyleContext = { width: 1280, height: 480 };
const PRINT: StyleContext = { width: 1280, print: true };
const CONTEXTS = { desk: DESKTOP, phone: PHONE, short: SHORT, print: PRINT } as const;

afterEach(() => {
  document.documentElement.removeAttribute("data-theme");
  document.body.innerHTML = "";
});

function rootTokens(ctx: StyleContext): Record<string, string> {
  return Object.fromEntries([...declaredStyle(document.documentElement, ctx)].sort(([a], [b]) => a.localeCompare(b)));
}

describe("with the look OFF nothing changes (characterization)", () => {
  for (const theme of ["light", "dark"] as const) {
    for (const [name, ctx] of Object.entries(CONTEXTS)) {
      it(`<html>, <body> and .app-root keep today's values (${theme}, ${name})`, () => {
        document.documentElement.setAttribute("data-theme", theme);
        const root = document.createElement("div");
        root.className = "app-root";
        document.body.append(root);
        expect({
          html: rootTokens(ctx),
          body: resolvedStyle(document.body, ctx),
          appRoot: resolvedStyle(root, ctx)
        }).toMatchSnapshot();
      });
    }

    it(`the header icon squares keep today's values (${theme})`, () => {
      document.documentElement.setAttribute("data-theme", theme);
      const { container } = render(
        <>
          <ModuleHeaderIcon icon="students" color="forest" />
          <ModuleHeaderIcon icon="students" color="yellow" />
          <ModuleHeaderIcon icon="students" color="neutral" />
          <ModuleHeaderIcon icon="students" color="purple" glow={false} />
        </>
      );
      expect([...container.children].map((el) => resolvedStyle(el, DESKTOP))).toMatchSnapshot();
    });

    it(`a keyboard-focused control keeps today's focus ring (${theme})`, () => {
      document.documentElement.setAttribute("data-theme", theme);
      const button = document.createElement("button");
      document.body.append(button);
      markFocusVisible(button);
      expect(resolvedStyle(button, DESKTOP)).toMatchSnapshot();
    });
  }
});

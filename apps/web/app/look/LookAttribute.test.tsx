import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import { render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LOOK_STORAGE_KEY } from "./look";
import { LookAttribute } from "./LookAttribute";

const html = (): HTMLElement => document.documentElement;

function renderWith(flags: FlagSet, resolved: boolean) {
  return render(
    <FlagsProvider flags={flags}>
      <LookAttribute resolved={resolved} />
    </FlagsProvider>
  );
}

afterEach(() => {
  localStorage.clear();
  html().removeAttribute("data-look");
});

describe("<LookAttribute/>", () => {
  it("puts data-look=refresh on <html> when module_ui_refresh is ON, and remembers it", () => {
    renderWith({ module_ui_refresh: true }, true);
    expect(html().getAttribute("data-look")).toBe("refresh");
    expect(localStorage.getItem(LOOK_STORAGE_KEY)).toBe("refresh");
  });

  it("clears it when the flag is OFF, and remembers that too", () => {
    html().setAttribute("data-look", "refresh");
    renderWith({}, true);
    expect(html().hasAttribute("data-look")).toBe(false);
    expect(localStorage.getItem(LOOK_STORAGE_KEY)).toBe("off");
  });

  it("follows the flag when it changes", () => {
    const { rerender } = renderWith({}, true);
    expect(html().hasAttribute("data-look")).toBe(false);
    rerender(
      <FlagsProvider flags={{ module_ui_refresh: true }}>
        <LookAttribute resolved />
      </FlagsProvider>
    );
    expect(html().getAttribute("data-look")).toBe("refresh");
  });

  // Until /features answers, the flags are the OFF fallback. Acting on them
  // would undo what the pre-paint script applied and flash the old look.
  it("keeps the pre-paint look untouched until the flags are resolved", () => {
    html().setAttribute("data-look", "refresh");
    localStorage.setItem(LOOK_STORAGE_KEY, "refresh");
    renderWith({}, false);
    expect(html().getAttribute("data-look")).toBe("refresh");
    expect(localStorage.getItem(LOOK_STORAGE_KEY)).toBe("refresh");
  });

  it("renders nothing", () => {
    const { container } = renderWith({ module_ui_refresh: true }, true);
    expect(container).toBeEmptyDOMElement();
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { prePaintScript } from "../theme/pre-paint";
import { THEME_STORAGE_KEY } from "../theme/theme-script";
import { LOOK_STORAGE_KEY, applyLook, lookBootstrapScript } from "./look";

/*
 * PLAN-redesign-home §3.3 "Cómo se enciende" (review E7): the look must be on
 * <html> before the first paint, or every load flashes the old look while
 * /api/v1/features is in flight. The last value the gateway gave is kept in
 * localStorage and replayed by the same pre-paint script that sets the theme.
 */

const html = (): HTMLElement => document.documentElement;
const run = (script: string): void => new Function(script)() as void;

afterEach(() => {
  localStorage.clear();
  html().removeAttribute("data-look");
  html().removeAttribute("data-theme");
  vi.restoreAllMocks();
});

describe("look bootstrap (pre-paint)", () => {
  it("applies the refresh look when the last value was refresh", () => {
    localStorage.setItem(LOOK_STORAGE_KEY, "refresh");
    run(lookBootstrapScript);
    expect(html().getAttribute("data-look")).toBe("refresh");
  });

  it("leaves today's look alone when nothing or anything else was stored", () => {
    run(lookBootstrapScript);
    expect(html().hasAttribute("data-look")).toBe(false);
    localStorage.setItem(LOOK_STORAGE_KEY, "off");
    run(lookBootstrapScript);
    expect(html().hasAttribute("data-look")).toBe(false);
  });

  it("never throws when storage is blocked (private mode)", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(() => run(lookBootstrapScript)).not.toThrow();
    expect(html().hasAttribute("data-look")).toBe(false);
  });

  it("is part of the one pre-paint script that also sets the theme", () => {
    localStorage.setItem(THEME_STORAGE_KEY, "dark");
    localStorage.setItem(LOOK_STORAGE_KEY, "refresh");
    run(prePaintScript);
    expect(html().getAttribute("data-theme")).toBe("dark");
    expect(html().getAttribute("data-look")).toBe("refresh");
  });

  it("a storage failure in the look does not cost the theme", () => {
    const real = Storage.prototype.getItem;
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(function (this: Storage, key: string) {
      if (key === LOOK_STORAGE_KEY) throw new Error("blocked");
      return real.call(this, key);
    });
    localStorage.setItem(THEME_STORAGE_KEY, "dark");
    run(prePaintScript);
    expect(html().getAttribute("data-theme")).toBe("dark");
  });
});

describe("applyLook", () => {
  it("sets the attribute and remembers refresh", () => {
    applyLook(true);
    expect(html().getAttribute("data-look")).toBe("refresh");
    expect(localStorage.getItem(LOOK_STORAGE_KEY)).toBe("refresh");
  });

  it("removes the attribute and remembers off", () => {
    applyLook(true);
    applyLook(false);
    expect(html().hasAttribute("data-look")).toBe(false);
    expect(localStorage.getItem(LOOK_STORAGE_KEY)).toBe("off");
  });

  it("still switches the look when storage is blocked", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(() => applyLook(true)).not.toThrow();
    expect(html().getAttribute("data-look")).toBe("refresh");
  });
});

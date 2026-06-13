/** localStorage key for the persisted theme (per design handoff). */
export const THEME_STORAGE_KEY = "gl-theme";

export type Theme = "light" | "dark";

export const DEFAULT_THEME: Theme = "light";

/**
 * Pre-hydration script (string) injected into <head>. It reads the persisted
 * theme and applies `data-theme` to <html> BEFORE first paint, so there is no
 * theme flash and no React hydration mismatch. Kept dependency-free and tiny.
 */
export const themeBootstrapScript = `(function () {
  try {
    var t = localStorage.getItem("${THEME_STORAGE_KEY}");
    if (t !== "dark" && t !== "light") t = "${DEFAULT_THEME}";
    document.documentElement.setAttribute("data-theme", t);
  } catch (e) {
    document.documentElement.setAttribute("data-theme", "${DEFAULT_THEME}");
  }
})();`;

/**
 * The app-wide look ("Captions adaptado", PLAN-redesign-home F2b). Its styles
 * live in globals.css under `html[data-look="refresh"]`; this module only owns
 * the attribute and its memory.
 */

/** localStorage key holding the last look the gateway granted. */
export const LOOK_STORAGE_KEY = "gl-look";

const LOOK_ATTRIBUTE = "data-look";
const REFRESH = "refresh";
const OFF = "off";

/**
 * Pre-paint snippet (string) for the <head> script: replays the remembered look
 * before the first paint, so a load does not flash the old look while
 * /api/v1/features is in flight. Anything but "refresh" leaves today's look.
 */
export const lookBootstrapScript = `(function () {
  try {
    if (localStorage.getItem("${LOOK_STORAGE_KEY}") === "${REFRESH}") {
      document.documentElement.setAttribute("${LOOK_ATTRIBUTE}", "${REFRESH}");
    }
  } catch (e) {}
})();`;

/** Switches <html> to (or off) the refresh look and remembers the choice. */
export function applyLook(refresh: boolean): void {
  const root = document.documentElement;
  if (refresh) {
    root.setAttribute(LOOK_ATTRIBUTE, REFRESH);
  } else {
    root.removeAttribute(LOOK_ATTRIBUTE);
  }
  try {
    localStorage.setItem(LOOK_STORAGE_KEY, refresh ? REFRESH : OFF);
  } catch {
    // Remembering is best-effort (private mode): the next load just starts
    // from today's look until /features answers.
  }
}

/**
 * Whether <html> wears the refresh look right now: the remembered look the
 * pre-paint script replayed, or the one applyLook set since. Readers that must
 * agree with the skin before /features answers ask this (review I1).
 */
export function isRefreshLookShown(): boolean {
  if (typeof document === "undefined") return false;
  return document.documentElement.getAttribute(LOOK_ATTRIBUTE) === REFRESH;
}

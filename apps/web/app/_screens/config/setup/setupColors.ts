/*
 * Colours of the setup view's small (13 px) text, held to WCAG AA 4.5:1 in
 * both themes (setupContrast.test.tsx). Built from the existing tokens only:
 * the global ones are shared by every screen and are not changed here.
 */

/**
 * A step's line when something is missing. `--warn-fg` alone is 4.1:1 on white;
 * mixing in `--text` darkens it in the light theme and lightens it in the dark
 * one, so it keeps reading as amber and passes in both.
 */
export const PENDING_TEXT = "color-mix(in oklab, var(--warn-fg) 70%, var(--text))";

/** Eyebrows and helper lines. `--text-faint` is 3.0:1 on white; `--text-muted` passes in both themes. */
export const QUIET_TEXT = "var(--text-muted)";

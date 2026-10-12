import type { CSSProperties, ReactElement } from "react";
import { COLOR, type ColorKey } from "./colors";
import { Icon, type IconName } from "./Icon";

export type ModuleHeaderIconProps = {
  icon: IconName;
  /** The square's color. `neutral` draws the grey square with no halo. */
  color: ColorKey;
  /** Glyph color over the square; defaults to the color's own ink. */
  ink?: string;
  /** The colored halo under the square (the `--shadow-icon` family). */
  glow?: boolean;
  /**
   * Legacy: half the headers never pinned the square's width, so on a very
   * narrow row (≈400% zoom) it squeezes. Kept so this extraction changes
   * nothing; the responsive matrix (PLAN-redesign-home F4) decides.
   */
  shrinks?: boolean;
};

/** Lime and yellow are light: their halo needs a little more ink to show. */
const STRONG_GLOW: ReadonlySet<ColorKey> = new Set(["lime", "yellow"]);

type IconVars = CSSProperties & Record<`--${string}`, string>;

/**
 * The 56px colored square at the head of every module screen. Its look lives
 * in globals.css (.module-header-icon), so a look layer can restyle all of
 * them at once; only the color goes inline, as custom properties.
 */
export function ModuleHeaderIcon({ icon, color, ink, glow = true, shrinks = false }: ModuleHeaderIconProps): ReactElement {
  const c = COLOR[color];
  const vars: IconVars = {
    "--icon-c": c.v,
    "--icon-ink": ink ?? c.ink,
    "--icon-glow": STRONG_GLOW.has(color) ? "40%" : "38%"
  };
  const classes = [
    "module-header-icon",
    color === "neutral" && "module-header-icon--neutral",
    !glow && "module-header-icon--flat",
    shrinks && "module-header-icon--shrinks"
  ].filter(Boolean).join(" ");
  return (
    <div className={classes} style={vars}>
      <Icon name={icon} size={30} />
    </div>
  );
}

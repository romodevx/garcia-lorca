/**
 * Module color → CSS variable + readable glyph ink for solid chips.
 * Ported from the design handoff (project/shared.jsx `COLOR`).
 */
export type ColorKey = "lime" | "forest" | "yellow" | "orange" | "purple" | "red" | "neutral";

export interface ColorToken {
  /** CSS value (a custom property reference). */
  v: string;
  /** Readable ink color for text/icons placed on the solid color. */
  ink: string;
}

export const COLOR: Record<ColorKey, ColorToken> = {
  lime: { v: "var(--lime)", ink: "#0C3318" },
  forest: { v: "var(--forest)", ink: "#FFFFFF" },
  yellow: { v: "var(--yellow)", ink: "#3A2E00" },
  orange: { v: "var(--orange)", ink: "#FFFFFF" },
  purple: { v: "var(--purple)", ink: "#FFFFFF" },
  red: { v: "var(--red)", ink: "#FFFFFF" },
  neutral: { v: "var(--text-muted)", ink: "#FFFFFF" }
};

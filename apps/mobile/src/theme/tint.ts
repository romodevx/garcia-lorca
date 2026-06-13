import type { ThemeColors, TintName } from "./tokens";

export interface TintPair {
  /** Soft background behind the icon tile. */
  bg: string;
  /** Icon / foreground ink. */
  fg: string;
}

/**
 * Resolves a tint name to its background + foreground pair, mirroring the
 * `.tint-*` CSS utilities (which were theme-aware: dark themes used a lighter
 * foreground ink for legibility).
 */
export function tintColors(colors: ThemeColors, tint: TintName): TintPair {
  switch (tint) {
    case "lime":
      return { bg: colors.primarySoft, fg: colors.tintLimeInk };
    case "forest":
      return { bg: colors.forestSoft, fg: colors.tintForestInk };
    case "yellow":
      return { bg: colors.yellowSoft, fg: colors.tintYellowInk };
    case "orange":
      return { bg: colors.orangeSoft, fg: colors.tintOrangeInk };
    case "red":
      return { bg: colors.redSoft, fg: colors.tintRedInk };
    case "purple":
      return { bg: colors.purpleSoft, fg: colors.tintPurpleInk };
  }
}

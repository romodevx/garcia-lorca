import type { ChipKind } from "../../data/mock";
import type { ThemeColors } from "../../theme";

/** Background/foreground pair for a status kind — the same palette the Chip uses. */
export function chipTint(colors: ThemeColors, kind: ChipKind): { bg: string; fg: string } {
  switch (kind) {
    case "ok":
      return { bg: colors.okBg, fg: colors.ok };
    case "warn":
      return { bg: colors.warnBg, fg: colors.warn };
    case "bad":
      return { bg: colors.badBg, fg: colors.bad };
    case "purple":
      return { bg: colors.purpleSoft, fg: colors.tintPurpleInk };
    case "lime":
      return { bg: colors.primarySoft, fg: colors.primaryInk };
    case "neutral":
      return { bg: colors.surfaceSunken, fg: colors.text2 };
  }
}

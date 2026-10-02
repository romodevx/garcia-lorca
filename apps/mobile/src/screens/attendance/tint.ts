import type { ChipKind } from "../../data/mock";
import type { ThemeColors } from "../../theme";
import { chipColors } from "../../ui/Chip";

/** Background/foreground pair for a status kind — the same palette the Chip uses. */
export function chipTint(colors: ThemeColors, kind: ChipKind): { bg: string; fg: string } {
  return chipColors(colors, kind);
}

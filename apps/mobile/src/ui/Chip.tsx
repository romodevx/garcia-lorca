import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { ChipKind } from "../data/mock";
import { Icon, type IconName } from "../icons";
import { useTheme, type ThemeColors } from "../theme";

export interface ChipProps {
  kind?: ChipKind;
  icon?: IconName;
  children: string;
}

function chipColors(colors: ThemeColors, kind: ChipKind): { bg: string; fg: string } {
  switch (kind) {
    case "ok":
      return { bg: colors.okBg, fg: colors.ok };
    case "warn":
      return { bg: colors.warnBg, fg: colors.warn };
    case "bad":
      return { bg: colors.badBg, fg: colors.bad };
    case "lime":
      return { bg: colors.primarySoft, fg: colors.primaryInk };
    case "purple":
      return { bg: colors.purpleSoft, fg: colors.tintPurpleInk };
    case "neutral":
      return { bg: colors.surfaceSunken, fg: colors.text2 };
  }
}

/** Pill-shaped status chip with optional leading icon. */
export function Chip({ kind = "neutral", icon, children }: ChipProps): ReactElement {
  const theme = useTheme();
  const { bg, fg } = chipColors(theme.colors, kind);
  return (
    <View style={[styles.chip, { backgroundColor: bg }]}>
      {icon ? <Icon name={icon} size={14} stroke={2.4} color={fg} /> : null}
      <Text style={[styles.text, { color: fg, fontFamily: theme.fonts.uiExtra }]}>{children}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  chip: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    height: 27,
    paddingHorizontal: 11,
    borderRadius: 999,
    alignSelf: "flex-start",
  },
  text: { fontSize: 12.5, fontWeight: "800" },
});

import { type ReactElement } from "react";
import { View, type StyleProp, type ViewStyle } from "react-native";
import { useTheme } from "../theme";

export interface ProgressBarProps {
  /** Fill 0–1. */
  pct: number;
  color: string;
  style?: StyleProp<ViewStyle>;
}

/** Thin rounded progress bar (qualitative term progress). */
export function ProgressBar({ pct, color, style }: ProgressBarProps): ReactElement {
  const theme = useTheme();
  const clamped = Math.max(0, Math.min(1, pct));
  return (
    <View
      style={[
        { height: 8, borderRadius: 999, backgroundColor: theme.colors.surfaceSunken, overflow: "hidden" },
        style,
      ]}
    >
      <View style={{ height: "100%", borderRadius: 999, backgroundColor: color, width: `${clamped * 100}%` }} />
    </View>
  );
}

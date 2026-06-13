import { type ReactElement } from "react";
import { StyleSheet, Text, View, type StyleProp, type TextStyle } from "react-native";
import { useTheme } from "../theme";

export interface SectionLabelProps {
  children: string;
  style?: StyleProp<TextStyle>;
}

/** Moderno section heading: big, bold, not all-caps. */
export function SectionLabel({ children, style }: SectionLabelProps): ReactElement {
  const theme = useTheme();
  return (
    <Text
      style={[
        styles.section,
        { color: theme.colors.text, fontFamily: theme.fonts.uiExtra },
        style,
      ]}
    >
      {children}
    </Text>
  );
}

export interface ScreenTitleProps {
  children: string;
}

/** Large screen title (detail-title scale). */
export function ScreenTitle({ children }: ScreenTitleProps): ReactElement {
  const theme = useTheme();
  return (
    <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
      {children}
    </Text>
  );
}

export interface StatusDotProps {
  color: string;
  size?: number;
}

/** Traffic-light status dot. */
export function StatusDot({ color, size = 10 }: StatusDotProps): ReactElement {
  return (
    <View style={{ width: size, height: size, borderRadius: size / 2, backgroundColor: color }} />
  );
}

const styles = StyleSheet.create({
  section: { fontSize: 18, fontWeight: "800", letterSpacing: -0.3, marginTop: 24, marginBottom: 12, marginHorizontal: 2 },
  title: { fontSize: 27, fontWeight: "800", letterSpacing: -0.6 },
});

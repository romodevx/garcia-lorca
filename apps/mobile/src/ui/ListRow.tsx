import { type ReactElement, type ReactNode } from "react";
import { Pressable, StyleSheet, View, type StyleProp, type ViewStyle } from "react-native";
import { useTheme } from "../theme";

export interface ListRowProps {
  children: ReactNode;
  onPress?: () => void;
  /** Draw a hairline separator above this row (used for all but the first). */
  divider?: boolean;
  style?: StyleProp<ViewStyle>;
  accessibilityLabel?: string;
  /** What pressing the row does, read after the label ("Ver todos los meses"). */
  accessibilityHint?: string;
}

/**
 * A single list row inside a Card. Rows draw their own top hairline (except the
 * first) to reproduce the prototype's inset separators.
 */
export function ListRow({
  children,
  onPress,
  divider = false,
  style,
  accessibilityLabel,
  accessibilityHint,
}: ListRowProps): ReactElement {
  const theme = useTheme();
  const content = (
    <View style={[styles.row, style]}>
      {divider ? (
        <View style={[styles.divider, { backgroundColor: theme.colors.line }]} />
      ) : null}
      {children}
    </View>
  );

  if (onPress) {
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={accessibilityLabel}
        accessibilityHint={accessibilityHint}
        onPress={onPress}
        android_ripple={{ color: theme.colors.surface2 }}
      >
        {content}
      </Pressable>
    );
  }
  return content;
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: 13,
    paddingVertical: 15,
    paddingHorizontal: 16,
  },
  divider: { position: "absolute", top: 0, left: 16, right: 16, height: 1 },
});

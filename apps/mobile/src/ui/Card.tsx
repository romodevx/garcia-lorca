import { type ReactElement, type ReactNode } from "react";
import {
  Pressable,
  StyleSheet,
  View,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { useTheme } from "../theme";

export interface CardProps {
  children: ReactNode;
  /** Inner padding (Moderno cards often pad their own rows, so this is opt-in). */
  padded?: boolean;
  /** When set, the whole card is a tappable target with press feedback. */
  onPress?: () => void;
  style?: StyleProp<ViewStyle>;
  accessibilityLabel?: string;
}

/**
 * Flat Moderno surface: hairline border, no shadow, large radius. Renders as a
 * Pressable when `onPress` is provided (scale-down press affordance).
 */
export function Card({
  children,
  padded = false,
  onPress,
  style,
  accessibilityLabel,
}: CardProps): ReactElement {
  const theme = useTheme();
  const base: StyleProp<ViewStyle> = [
    styles.card,
    {
      backgroundColor: theme.colors.surface,
      borderColor: theme.colors.line,
      borderRadius: theme.radius.card,
    },
    padded ? styles.padded : null,
    style,
  ];

  if (onPress) {
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={accessibilityLabel}
        onPress={onPress}
        style={({ pressed }) => [base, pressed ? styles.pressed : null]}
      >
        {children}
      </Pressable>
    );
  }

  return (
    <View accessibilityLabel={accessibilityLabel} style={base}>
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { borderWidth: 1, overflow: "hidden" },
  padded: { padding: 16 },
  pressed: { transform: [{ scale: 0.985 }] },
});

import { type ReactElement } from "react";
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  View,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { Icon, type IconName } from "../icons";
import { useTheme, type ThemeColors } from "../theme";

export type ButtonVariant = "primary" | "ghost" | "outline" | "danger";

export interface ButtonProps {
  label: string;
  onPress?: () => void;
  variant?: ButtonVariant;
  icon?: IconName;
  /** Smaller height (44 vs 54). */
  small?: boolean;
  disabled?: boolean;
  loading?: boolean;
  style?: StyleProp<ViewStyle>;
  accessibilityLabel?: string;
}

interface VariantStyle {
  bg: string;
  fg: string;
  borderColor?: string;
}

function variantColors(colors: ThemeColors, variant: ButtonVariant, pressed: boolean): VariantStyle {
  switch (variant) {
    case "primary":
      return { bg: pressed ? colors.primaryPress : colors.primary, fg: colors.onAccent };
    case "ghost":
      return { bg: colors.surfaceSunken, fg: colors.text };
    case "outline":
      return { bg: "transparent", fg: colors.text, borderColor: colors.lineStrong };
    case "danger":
      return { bg: colors.bad, fg: "#FFFFFF" };
  }
}

/** App-like flat button. Primary uses lime + dark ink; danger uses red. */
export function Button({
  label,
  onPress,
  variant = "primary",
  icon,
  small = false,
  disabled = false,
  loading = false,
  style,
  accessibilityLabel,
}: ButtonProps): ReactElement {
  const theme = useTheme();
  const isInactive = disabled || loading;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? label}
      accessibilityState={{ disabled: isInactive }}
      disabled={isInactive}
      onPress={onPress}
      style={({ pressed }) => {
        const v = variantColors(theme.colors, variant, pressed);
        return [
          styles.button,
          small ? styles.small : styles.large,
          {
            backgroundColor: v.bg,
            borderColor: v.borderColor ?? "transparent",
            borderWidth: variant === "outline" ? 1.6 : 0,
            opacity: isInactive ? 0.5 : 1,
            transform: [{ scale: pressed ? 0.975 : 1 }],
          },
          style,
        ];
      }}
    >
      {({ pressed }) => {
        const v = variantColors(theme.colors, variant, pressed);
        return (
          <View style={styles.content}>
            {loading ? (
              <ActivityIndicator color={v.fg} />
            ) : (
              <>
                {icon ? <Icon name={icon} size={small ? 18 : 20} stroke={2.2} color={v.fg} /> : null}
                <Text
                  style={[
                    styles.label,
                    { color: v.fg, fontFamily: theme.fonts.uiExtra, fontSize: small ? 15 : 17 },
                  ]}
                >
                  {label}
                </Text>
              </>
            )}
          </View>
        );
      }}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: {
    width: "100%",
    alignItems: "center",
    justifyContent: "center",
  },
  large: { height: 54, borderRadius: 16 },
  small: { height: 44, borderRadius: 13 },
  content: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 9 },
  label: { fontWeight: "800", letterSpacing: -0.1 },
});

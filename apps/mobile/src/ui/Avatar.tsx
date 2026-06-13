import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { Child } from "../data/mock";
import { useTheme } from "../theme";

export interface AvatarProps {
  child: Child;
  size?: number;
  /** Moderno skin uses a circular story-ring avatar. */
  ring?: boolean;
}

/**
 * Initials avatar with the child's brand gradient. In the Moderno skin it is a
 * circle wrapped in a lime "story ring" (a ring separated from the photo by a
 * background-colored gap), matching the design handoff.
 */
export function Avatar({ child, size = 44, ring = true }: AvatarProps): ReactElement {
  const theme = useTheme();
  const inner = (
    <View
      style={[
        styles.face,
        {
          width: size,
          height: size,
          borderRadius: size / 2,
          backgroundColor: child.avatar.from,
        },
      ]}
    >
      {/* Approximate the diagonal gradient with the two brand stops layered. */}
      <View
        style={[
          StyleSheet.absoluteFill,
          { borderRadius: size / 2, backgroundColor: child.avatar.to, opacity: 0.55 },
        ]}
      />
      <Text style={[styles.initials, { fontSize: size * 0.38, fontFamily: theme.fonts.display }]}>
        {child.initials}
      </Text>
    </View>
  );

  if (!ring) return inner;

  return (
    <View
      style={{
        padding: 4,
        borderRadius: (size + 8) / 2,
        backgroundColor: theme.colors.primary,
      }}
    >
      <View
        style={{
          padding: 2,
          borderRadius: (size + 4) / 2,
          backgroundColor: theme.colors.bg,
        }}
      >
        {inner}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  face: { alignItems: "center", justifyContent: "center", overflow: "hidden" },
  initials: { color: "#FFFFFF", fontWeight: "700", letterSpacing: 0.5 },
});

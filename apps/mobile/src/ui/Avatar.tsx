import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { useTheme } from "../theme";

/** Default brand gradient stops (lime → forest) when a child carries no photo. */
const BRAND_FROM = "#7AC400";
const BRAND_TO = "#0A7A2F";

export interface AvatarProps {
  /** Two-letter monogram (empty renders a blank brand circle). */
  initials: string;
  size?: number;
  /** Moderno skin uses a circular story-ring avatar. */
  ring?: boolean;
  /** Gradient start stop. */
  from?: string;
  /** Gradient end stop. */
  to?: string;
}

/**
 * Initials avatar with a brand gradient. In the Moderno skin it is a circle
 * wrapped in a lime "story ring" (a ring separated from the photo by a
 * background-colored gap), matching the design handoff.
 */
export function Avatar({
  initials,
  size = 44,
  ring = true,
  from = BRAND_FROM,
  to = BRAND_TO,
}: AvatarProps): ReactElement {
  const theme = useTheme();
  const inner = (
    <View
      style={[
        styles.face,
        {
          width: size,
          height: size,
          borderRadius: size / 2,
          backgroundColor: from,
        },
      ]}
    >
      {/* Approximate the diagonal gradient with the two brand stops layered. */}
      <View
        style={[
          StyleSheet.absoluteFill,
          { borderRadius: size / 2, backgroundColor: to, opacity: 0.55 },
        ]}
      />
      <Text style={[styles.initials, { fontSize: size * 0.38, fontFamily: theme.fonts.display }]}>
        {initials}
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

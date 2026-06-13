import { type ReactElement } from "react";
import { View } from "react-native";
import { Icon, type IconName } from "../icons";
import { tintColors, useTheme, type TintName } from "../theme";

export interface TintIconProps {
  name: IconName;
  tint?: TintName;
  size?: number;
  /** Inner icon glyph size. */
  icon?: number;
  /** Moderno tiles are circular; pass false for a rounded-square tile. */
  round?: boolean;
}

/** Rounded line icon inside a soft tinted tile (Moderno: circular). */
export function TintIcon({
  name,
  tint = "lime",
  size = 40,
  icon = 22,
  round = true,
}: TintIconProps): ReactElement {
  const theme = useTheme();
  const { bg, fg } = tintColors(theme.colors, tint);
  return (
    <View
      style={{
        width: size,
        height: size,
        borderRadius: round ? size / 2 : theme.radius.sm,
        backgroundColor: bg,
        alignItems: "center",
        justifyContent: "center",
        flexShrink: 0,
      }}
    >
      <Icon name={name} size={icon} color={fg} />
    </View>
  );
}

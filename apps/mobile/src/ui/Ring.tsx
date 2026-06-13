import { type ReactElement, type ReactNode } from "react";
import { View } from "react-native";
import Svg, { Circle } from "react-native-svg";

export interface RingProps {
  /** Progress 0–1. */
  pct: number;
  color: string;
  track: string;
  size?: number;
  /** Stroke width. */
  sw?: number;
  children?: ReactNode;
}

/** SVG ring progress indicator (used for the grade average). */
export function Ring({ pct, color, track, size = 92, sw = 9, children }: RingProps): ReactElement {
  const r = (size - sw) / 2;
  const c = 2 * Math.PI * r;
  const clamped = Math.max(0, Math.min(1, pct));
  return (
    <View style={{ width: size, height: size, alignItems: "center", justifyContent: "center" }}>
      <Svg width={size} height={size} style={{ position: "absolute" }}>
        <Circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={track} strokeWidth={sw} />
        <Circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={color}
          strokeWidth={sw}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - clamped)}
          // Start at 12 o'clock (rotate -90° around centre).
          rotation={-90}
          originX={size / 2}
          originY={size / 2}
        />
      </Svg>
      <View style={{ alignItems: "center" }}>{children}</View>
    </View>
  );
}

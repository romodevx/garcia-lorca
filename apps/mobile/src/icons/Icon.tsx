import type { ReactElement } from "react";
import Svg, { Circle, Path, Rect } from "react-native-svg";

/**
 * Rounded line-icon set — one consistent family (stroke 2, round caps), ported
 * from the design handoff (icons.jsx) to react-native-svg. Icons inherit the
 * passed `color` (the RN equivalent of `currentColor`).
 */
export type IconName =
  | "home"
  | "calendar"
  | "calendar-check"
  | "award"
  | "chart"
  | "wallet"
  | "megaphone"
  | "grid"
  | "bell"
  | "chevron-right"
  | "chevron-left"
  | "chevron-down"
  | "check"
  | "check-circle"
  | "x"
  | "x-circle"
  | "plus"
  | "clock"
  | "document"
  | "shirt"
  | "receipt"
  | "cap"
  | "user"
  | "sun"
  | "moon"
  | "refresh"
  | "alert"
  | "arrow-right"
  | "arrow-up-right"
  | "lock"
  | "eye"
  | "eye-off"
  | "inbox"
  | "sparkles"
  | "heart"
  | "pin"
  | "book"
  | "phone"
  | "map"
  | "star"
  | "minus"
  | "download"
  | "credit-card"
  | "id-card"
  | "flame"
  | "leaf";

export interface IconProps {
  name: IconName;
  size?: number;
  /** Stroke width; round joins/caps. */
  stroke?: number;
  /** Stroke + fill color (acts like CSS currentColor). */
  color?: string;
}

type Common = { strokeWidth: number; color: string };

/** Returns the path/shape children for a given icon name. */
function shapes(name: IconName, { strokeWidth, color }: Common): ReactElement {
  const s = {
    stroke: color,
    strokeWidth,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    fill: "none" as const,
  };
  switch (name) {
    case "home":
      return (
        <>
          <Path d="M4 11.5 12 4.5l8 7" {...s} />
          <Path d="M6 10.5V19a1 1 0 0 0 1 1h3.5v-4.5a1.5 1.5 0 0 1 3 0V20H17a1 1 0 0 0 1-1v-8.5" {...s} />
        </>
      );
    case "calendar":
      return (
        <>
          <Rect x={3.5} y={5} width={17} height={15.5} rx={3.5} {...s} />
          <Path d="M3.5 9.5h17M8 3.5v3M16 3.5v3" {...s} />
        </>
      );
    case "calendar-check":
      return (
        <>
          <Rect x={3.5} y={5} width={17} height={15.5} rx={3.5} {...s} />
          <Path d="M3.5 9.5h17M8 3.5v3M16 3.5v3" {...s} />
          <Path d="m8.8 14.5 2 2 3.6-3.8" {...s} />
        </>
      );
    case "award":
      return (
        <>
          <Circle cx={12} cy={9.5} r={5.5} {...s} />
          <Path d="m8.5 14 -1.3 6 4.8-2.6 4.8 2.6-1.3-6" {...s} />
        </>
      );
    case "chart":
      return (
        <>
          <Path d="M4.5 4.5v13a2 2 0 0 0 2 2h13" {...s} />
          <Path d="M8.5 15.5v-3M12.5 15.5v-6M16.5 15.5v-9" {...s} />
        </>
      );
    case "wallet":
      return (
        <>
          <Path d="M3.5 8a3 3 0 0 1 3-3h10.5a1.5 1.5 0 0 1 1.5 1.5V8" {...s} />
          <Rect x={3.5} y={7} width={17} height={12.5} rx={3.5} {...s} />
          <Path d="M20.5 12h-3.2a2 2 0 0 0 0 4h3.2" {...s} />
        </>
      );
    case "megaphone":
      return (
        <>
          <Path d="M4 10v4a1.5 1.5 0 0 0 1.5 1.5H7l1 4h2l-.7-4" {...s} />
          <Path d="M7 15.5V8.5l9.5-4v15z" {...s} />
          <Path d="M16.5 8.5a3 3 0 0 1 0 7" {...s} />
        </>
      );
    case "grid":
      return (
        <>
          <Rect x={4} y={4} width={6.5} height={6.5} rx={2} {...s} />
          <Rect x={13.5} y={4} width={6.5} height={6.5} rx={2} {...s} />
          <Rect x={4} y={13.5} width={6.5} height={6.5} rx={2} {...s} />
          <Rect x={13.5} y={13.5} width={6.5} height={6.5} rx={2} {...s} />
        </>
      );
    case "bell":
      return (
        <>
          <Path d="M6 10a6 6 0 0 1 12 0c0 4 1.2 5.2 2 6H4c.8-.8 2-2 2-6Z" {...s} />
          <Path d="M9.5 20a2.5 2.5 0 0 0 5 0" {...s} />
        </>
      );
    case "chevron-right":
      return <Path d="m9.5 5.5 6.5 6.5-6.5 6.5" {...s} />;
    case "chevron-left":
      return <Path d="m14.5 5.5-6.5 6.5 6.5 6.5" {...s} />;
    case "chevron-down":
      return <Path d="m5.5 9.5 6.5 6.5 6.5-6.5" {...s} />;
    case "check":
      return <Path d="m5 12.5 4.5 4.5L19 7" {...s} />;
    case "check-circle":
      return (
        <>
          <Circle cx={12} cy={12} r={8.5} {...s} />
          <Path d="m8.5 12 2.4 2.4 4.6-4.8" {...s} />
        </>
      );
    case "x":
      return <Path d="M6.5 6.5 17.5 17.5M17.5 6.5 6.5 17.5" {...s} />;
    case "x-circle":
      return (
        <>
          <Circle cx={12} cy={12} r={8.5} {...s} />
          <Path d="M9.2 9.2 14.8 14.8M14.8 9.2 9.2 14.8" {...s} />
        </>
      );
    case "plus":
      return <Path d="M12 5.5v13M5.5 12h13" {...s} />;
    case "clock":
      return (
        <>
          <Circle cx={12} cy={12} r={8.5} {...s} />
          <Path d="M12 7.5V12l3 2" {...s} />
        </>
      );
    case "document":
      return (
        <>
          <Path d="M6.5 3.5h7l5 5v11a1.5 1.5 0 0 1-1.5 1.5h-10.5a1.5 1.5 0 0 1-1.5-1.5V5a1.5 1.5 0 0 1 1.5-1.5Z" {...s} />
          <Path d="M13 3.5V8a1 1 0 0 0 1 1h4.5" {...s} />
          <Path d="M8.5 13h7M8.5 16.5h5" {...s} />
        </>
      );
    case "shirt":
      return (
        <>
          <Path d="M8.5 4 5 6 3.5 9l2.5 1.5" {...s} />
          <Path d="M15.5 4 19 6l1.5 3-2.5 1.5" {...s} />
          <Path d="M8.5 4c0 1.8 1.4 3 3.5 3s3.5-1.2 3.5-3" {...s} />
          <Path d="M6 10.5V20h12v-9.5" {...s} />
        </>
      );
    case "receipt":
      return (
        <>
          <Path d="M5.5 4.5h13v15.5l-2-1.4-2 1.4-2-1.4-1 .7-1-.7-2 1.4-2-1.4Z" {...s} />
          <Path d="M8.5 9h7M8.5 12.5h7" {...s} />
        </>
      );
    case "cap":
      return (
        <>
          <Path d="m12 4 9 4.5-9 4.5-9-4.5Z" {...s} />
          <Path d="M6.5 11v4.5c0 1.4 2.5 2.5 5.5 2.5s5.5-1.1 5.5-2.5V11" {...s} />
          <Path d="M21 8.5v5" {...s} />
        </>
      );
    case "user":
      return (
        <>
          <Circle cx={12} cy={8.5} r={3.8} {...s} />
          <Path d="M5 19.5a7 7 0 0 1 14 0" {...s} />
        </>
      );
    case "sun":
      return (
        <>
          <Circle cx={12} cy={12} r={4} {...s} />
          <Path d="M12 2.5v2.5M12 19v2.5M4.2 4.2l1.8 1.8M18 18l1.8 1.8M2.5 12H5M19 12h2.5M4.2 19.8 6 18M18 6l1.8-1.8" {...s} />
        </>
      );
    case "moon":
      return <Path d="M19 13.5A7.5 7.5 0 0 1 10.5 5a6 6 0 1 0 8.5 8.5Z" {...s} />;
    case "refresh":
      return (
        <>
          <Path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3" {...s} />
          <Path d="M19.5 4.5V9H15" {...s} />
        </>
      );
    case "alert":
      return (
        <>
          <Path d="M12 4.5 21 19.5H3Z" {...s} />
          <Path d="M12 10v4M12 16.8v.2" {...s} />
        </>
      );
    case "arrow-right":
      return <Path d="M5 12h13M13 6.5 18.5 12 13 17.5" {...s} />;
    case "arrow-up-right":
      return <Path d="M7 17 17 7M9 7h8v8" {...s} />;
    case "lock":
      return (
        <>
          <Rect x={5} y={10.5} width={14} height={10} rx={3} {...s} />
          <Path d="M8 10.5V8a4 4 0 0 1 8 0v2.5" {...s} />
          <Circle cx={12} cy={15.5} r={1.3} {...s} />
        </>
      );
    case "eye":
      return (
        <>
          <Path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z" {...s} />
          <Circle cx={12} cy={12} r={3} {...s} />
        </>
      );
    case "eye-off":
      return (
        <>
          <Path d="M4 4.5 20 19.5" {...s} />
          <Path d="M9.5 9.6A3 3 0 0 0 12 15c.5 0 1-.1 1.4-.4" {...s} />
          <Path d="M6.6 6.8C4.2 8.3 2.5 12 2.5 12s3.5 6.5 9.5 6.5c1.6 0 3-.4 4.2-1" {...s} />
          <Path d="M9.8 5.8A8 8 0 0 1 12 5.5c6 0 9.5 6.5 9.5 6.5a17 17 0 0 1-2.4 3.2" {...s} />
        </>
      );
    case "inbox":
      return (
        <>
          <Path d="M3.5 13.5 6 6a2 2 0 0 1 1.9-1.5h8.2A2 2 0 0 1 18 6l2.5 7.5" {...s} />
          <Path d="M3.5 13.5V18a1.5 1.5 0 0 0 1.5 1.5h14a1.5 1.5 0 0 0 1.5-1.5v-4.5h-5a3 3 0 0 1-6 0Z" {...s} />
        </>
      );
    case "sparkles":
      return (
        <>
          <Path d="M12 4.5 13.4 9 18 10.4 13.4 11.8 12 16.3 10.6 11.8 6 10.4 10.6 9Z" {...s} />
          <Path d="M18 4v3M19.5 5.5h-3" {...s} />
        </>
      );
    case "heart":
      return <Path d="M12 19.5S4.5 15 4.5 9.7A3.7 3.7 0 0 1 12 7.6a3.7 3.7 0 0 1 7.5 2.1C19.5 15 12 19.5 12 19.5Z" {...s} />;
    case "pin":
      return (
        <>
          <Path d="M12 21s6-5.3 6-10a6 6 0 0 0-12 0c0 4.7 6 10 6 10Z" {...s} />
          <Circle cx={12} cy={11} r={2.3} {...s} />
        </>
      );
    case "book":
      return (
        <>
          <Path d="M5 4.5h9a2 2 0 0 1 2 2v13a1.5 1.5 0 0 0-1.5-1.5H5Z" {...s} />
          <Path d="M5 4.5v13.5" {...s} />
          <Path d="M16 6.5h2.5a1 1 0 0 1 1 1V19a1.5 1.5 0 0 0-1.5-1.5H16" {...s} />
        </>
      );
    case "phone":
      return <Path d="M6 3.5h3l1.5 4-2 1.5a11 11 0 0 0 5 5l1.5-2 4 1.5v3a2 2 0 0 1-2 2A15.5 15.5 0 0 1 4 5.5a2 2 0 0 1 2-2Z" {...s} />;
    case "map":
      return (
        <>
          <Path d="m9 4.5-5 2v13l5-2 6 2 5-2v-13l-5 2-6-2Z" {...s} />
          <Path d="M9 4.5v15M15 6.5v15" {...s} />
        </>
      );
    case "star":
      return <Path d="m12 4 2.3 4.9 5.2.6-3.9 3.6 1.1 5.3L12 15.8 7.3 18.4l1.1-5.3L4.5 9.5l5.2-.6Z" {...s} />;
    case "minus":
      return <Path d="M5.5 12h13" {...s} />;
    case "download":
      return (
        <>
          <Path d="M12 4v10M7.5 10 12 14.5 16.5 10" {...s} />
          <Path d="M5 19.5h14" {...s} />
        </>
      );
    case "credit-card":
      return (
        <>
          <Rect x={3.5} y={5.5} width={17} height={13} rx={3} {...s} />
          <Path d="M3.5 10h17M7 15h3" {...s} />
        </>
      );
    case "id-card":
      return (
        <>
          <Rect x={3} y={5} width={18} height={14} rx={3} {...s} />
          <Circle cx={8.5} cy={10.5} r={2} {...s} />
          <Path d="M5.8 15.5a2.8 2.8 0 0 1 5.4 0M13.5 10h4M13.5 13.5h3" {...s} />
        </>
      );
    case "flame":
      return <Path d="M12 3.5s4.5 3.5 4.5 8a4.5 4.5 0 0 1-9 0c0-1.2.5-2.2 1-2.8.2 1 .8 1.6 1.5 1.8-.3-2.4 2-4.6 2-7Z" {...s} />;
    case "leaf":
      return (
        <>
          <Path d="M5 19s-1-7 4-11 10-3 10-3 .5 7-3.5 11S5 19 5 19Z" {...s} />
          <Path d="M8 16c2-3 5-5 8-6" {...s} />
        </>
      );
  }
}

export function Icon({ name, size = 22, stroke = 2, color = "#000" }: IconProps): ReactElement {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" fill="none">
      {shapes(name, { strokeWidth: stroke, color })}
    </Svg>
  );
}

/**
 * Design tokens — ported from the design handoff (app.css + app-modern.css),
 * "Moderno" skin only (the default per the design decision). The web prototype
 * expressed these as CSS custom properties; here they are typed RN values.
 *
 * Brand palette comes from the school logo: lime is primary, red is alerts /
 * destructive ONLY. Two themes: light (airy white) and dark (deep forest with
 * luminous accents).
 */

/** Logo palette — shared across both themes. */
export const palette = {
  lime: "#7AC400",
  forest: "#0A7A2F",
  yellow: "#F2D600",
  orange: "#F39200",
  red: "#E00000",
  purple: "#8E2E8E",
} as const;

/** A semantic color set. Both themes implement the same keys. */
export interface ThemeColors {
  // surfaces / background
  bg: string;
  surface: string;
  surface2: string;
  surfaceSunken: string;
  line: string;
  lineStrong: string;
  // text
  text: string;
  text2: string;
  text3: string;
  onAccent: string;
  // primary (lime)
  primary: string;
  primaryPress: string;
  primarySoft: string;
  primaryInk: string;
  // soft tints (for icon tiles / accents)
  forestSoft: string;
  yellowSoft: string;
  orangeSoft: string;
  redSoft: string;
  purpleSoft: string;
  // traffic-light status
  ok: string;
  okBg: string;
  warn: string;
  warnBg: string;
  bad: string;
  badBg: string;
  // misc
  avatarRing: string;
  // tint icon foreground (theme-aware: dark needs lighter ink)
  tintLimeInk: string;
  tintForestInk: string;
  tintYellowInk: string;
  tintOrangeInk: string;
  tintRedInk: string;
  tintPurpleInk: string;
  // scrim for bottom sheets
  scrim: string;
}

/** Light theme — Moderno skin (flat white feed, hairline borders). */
export const lightColors: ThemeColors = {
  bg: "#FFFFFF",
  surface: "#FFFFFF",
  surface2: "#F2F5F1",
  surfaceSunken: "#EEF2ED",
  line: "#E7ECE5",
  lineStrong: "#DCE3D9",

  text: "#111A14",
  text2: "#5A6B5F",
  text3: "#8A988E",
  onAccent: "#0B3D1B",

  primary: "#6FB800",
  primaryPress: "#5EA000",
  primarySoft: "#EAF6D6",
  primaryInk: "#2E5400",

  forestSoft: "#DDEFE2",
  yellowSoft: "#FBF3C9",
  orangeSoft: "#FCE6CB",
  redSoft: "#FBDADA",
  purpleSoft: "#F3DEF3",

  ok: "#0A7A2F",
  okBg: "#DDEFE2",
  warn: "#C97A00",
  warnBg: "#FBEFD3",
  bad: "#C40000",
  badBg: "#FBDADA",

  avatarRing: "#FFFFFF",

  tintLimeInk: palette.forest,
  tintForestInk: palette.forest,
  tintYellowInk: "#C97A00",
  tintOrangeInk: palette.orange,
  tintRedInk: palette.red,
  tintPurpleInk: palette.purple,

  scrim: "rgba(4,20,10,0.42)",
};

/** Dark theme — Moderno skin (very deep forest, flat, clean contrast). */
export const darkColors: ThemeColors = {
  bg: "#051A0E",
  surface: "#0A2B17",
  surface2: "#0E3520",
  surfaceSunken: "#082413",
  line: "rgba(255,255,255,0.08)",
  lineStrong: "rgba(255,255,255,0.14)",

  text: "#ECF7EE",
  text2: "#9FBEA9",
  text3: "#6F9079",
  onAccent: "#06250F",

  primary: "#7AC400",
  primaryPress: "#8BD419",
  primarySoft: "rgba(122,196,0,0.16)",
  primaryInk: "#BFEF7E",

  forestSoft: "rgba(122,196,0,0.14)",
  yellowSoft: "rgba(242,214,0,0.16)",
  orangeSoft: "rgba(243,146,0,0.18)",
  redSoft: "rgba(224,0,0,0.20)",
  purpleSoft: "rgba(178,90,178,0.22)",

  ok: "#8DDB4F",
  okBg: "rgba(122,196,0,0.16)",
  warn: "#F2D600",
  warnBg: "rgba(242,214,0,0.15)",
  bad: "#FF6B6B",
  badBg: "rgba(224,0,0,0.20)",

  avatarRing: "rgba(255,255,255,0.14)",

  tintLimeInk: "#9DE24F",
  tintForestInk: "#6FD08A",
  tintYellowInk: "#F2D600",
  tintOrangeInk: "#FFB454",
  tintRedInk: "#FF8585",
  tintPurpleInk: "#D98BD9",

  scrim: "rgba(0,0,0,0.55)",
};

/** Spacing scale (dp). */
export const spacing = {
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 18,
  xxl: 22,
  xxxl: 26,
} as const;

/** Corner radii (dp). */
export const radius = {
  sm: 11,
  md: 14,
  lg: 16,
  xl: 18,
  card: 20,
  sheet: 28,
  pill: 999,
  full: 9999,
} as const;

/** Type scale (Moderno favours big, bold, slightly tight titles). */
export const typeScale = {
  greet: 12,
  childName: 20,
  sectionLabel: 18,
  detailTitle: 27,
  heroValue: 25,
  balanceAmount: 44,
  body: 15,
  sub: 12.5,
  amount: 16,
  tabLabel: 9.5,
} as const;

/** Font family roles. Wired to the loaded @expo-google-fonts families. */
export const fonts = {
  /** Baloo 2 — warm display for titles, numerals, balance. */
  display: "Baloo2_700Bold",
  displaySemi: "Baloo2_600SemiBold",
  /** Nunito — friendly rounded UI/body. */
  ui: "Nunito_700Bold",
  uiSemi: "Nunito_600SemiBold",
  uiExtra: "Nunito_800ExtraBold",
  uiRegular: "Nunito_500Medium",
} as const;

export type TintName = "lime" | "forest" | "yellow" | "orange" | "red" | "purple";

import { router } from "expo-router";
import type { BottomTabBarProps } from "expo-router/tabs";
import { type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { Icon, type IconName } from "../icons";
import { useAppState } from "../state/AppState";
import { useTheme } from "../theme";

interface TabMeta {
  name: string;
  label: string;
  icon: IconName;
}

/**
 * Tab order + labels (es-MX). The 6th "Más" entry is not a tab route — it pushes
 * the module-grid screen — but it renders in the same bar for parity with the
 * design's bottom navigation.
 */
const TABS: TabMeta[] = [
  { name: "inicio", label: "Inicio", icon: "home" },
  { name: "asistencia", label: "Asistencia", icon: "calendar-check" },
  { name: "calificaciones", label: "Notas", icon: "award" },
  { name: "cuenta", label: "Cuenta", icon: "wallet" },
  { name: "avisos", label: "Avisos", icon: "megaphone" },
];

/**
 * Moderno-skin bottom bar: flat with a hairline top border, an active-tab lime
 * pill behind the icon, and an unread badge on Avisos. "Más" sits at the end and
 * navigates to the module grid.
 */
export function TabBar({ state, navigation }: BottomTabBarProps): ReactElement {
  const theme = useTheme();
  const insets = useSafeAreaInsets();
  const { unreadCount } = useAppState();
  const activeRoute = state.routes[state.index]?.name;

  return (
    <View
      style={[
        styles.bar,
        {
          backgroundColor: theme.colors.bg,
          borderTopColor: theme.colors.line,
          paddingBottom: 8 + insets.bottom,
        },
      ]}
    >
      {TABS.map((tab) => {
        const focused = activeRoute === tab.name;
        return (
          <TabButton
            key={tab.name}
            tab={tab}
            focused={focused}
            badge={tab.name === "avisos" ? unreadCount : 0}
            onPress={() => {
              if (!focused) navigation.navigate(tab.name);
            }}
          />
        );
      })}
      <TabButton
        tab={{ name: "mas", label: "Más", icon: "grid" }}
        focused={false}
        badge={0}
        onPress={() => router.push("/mas")}
      />
    </View>
  );
}

interface TabButtonProps {
  tab: TabMeta;
  focused: boolean;
  badge: number;
  onPress: () => void;
}

function TabButton({ tab, focused, badge, onPress }: TabButtonProps): ReactElement {
  const theme = useTheme();
  const tint = focused ? (theme.name === "dark" ? theme.colors.primary : theme.palette.forest) : theme.colors.text3;
  return (
    <Pressable
      accessibilityRole="tab"
      accessibilityState={{ selected: focused }}
      accessibilityLabel={tab.label}
      onPress={onPress}
      style={styles.tab}
    >
      <View style={styles.iconWrap}>
        {focused ? (
          <View style={[styles.pill, { backgroundColor: theme.colors.primarySoft }]} />
        ) : null}
        <Icon name={tab.icon} size={23} stroke={focused ? 2.4 : 2} color={tint} />
        {badge > 0 ? (
          <View style={[styles.badge, { backgroundColor: theme.palette.red, borderColor: theme.colors.bg }]}>
            <Text style={styles.badgeText}>{badge}</Text>
          </View>
        ) : null}
      </View>
      <Text style={[styles.label, { color: tint, fontFamily: theme.fonts.uiExtra }]}>{tab.label}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  bar: { flexDirection: "row", borderTopWidth: StyleSheet.hairlineWidth, paddingTop: 8, paddingHorizontal: 6 },
  tab: { flex: 1, alignItems: "center", gap: 3, paddingVertical: 3 },
  iconWrap: { alignItems: "center", justifyContent: "center" },
  pill: { position: "absolute", top: -6, bottom: -6, left: -16, right: -16, borderRadius: 999 },
  label: { fontSize: 9.5, fontWeight: "800", letterSpacing: 0.1 },
  badge: {
    position: "absolute",
    top: -4,
    right: -7,
    minWidth: 16,
    height: 16,
    paddingHorizontal: 4,
    borderRadius: 8,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 2.5,
  },
  badgeText: { color: "#FFFFFF", fontSize: 10, fontWeight: "800" },
});

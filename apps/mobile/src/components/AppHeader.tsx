import { router } from "expo-router";
import { type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { Icon } from "../icons";
import { LanguageToggle } from "../i18n/LanguageToggle";
import { useAppState } from "../state/AppState";
import { toChildView } from "../state/childView";
import { useTheme, useThemeControls } from "../theme";
import { Avatar } from "../ui";

/**
 * Persistent app header carrying the single-child context (story-ring avatar,
 * greeting with group, child name) plus theme toggle and a bell that routes to
 * Avisos with an unread badge. Shown across the tab screens.
 */
export function AppHeader(): ReactElement {
  const theme = useTheme();
  const { toggleTheme } = useThemeControls();
  const { child, unreadCount } = useAppState();
  const view = toChildView(child);
  const insets = useSafeAreaInsets();

  return (
    <View
      style={[
        styles.header,
        { paddingTop: insets.top + 12, backgroundColor: theme.colors.bg, borderBottomColor: theme.colors.line },
      ]}
    >
      <View style={styles.id}>
        <Avatar initials={view.initials} size={40} />
        <View style={styles.idText}>
          <Text style={[styles.greet, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>
            {view.present ? view.subtitle : "García Lorca"}
          </Text>
          <Text
            style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}
            numberOfLines={2}
          >
            {view.present ? view.headerName : "Portal de familias"}
          </Text>
        </View>
      </View>

      <View style={styles.actions}>
        <LanguageToggle />
        <IconButton
          label="Cambiar tema"
          onPress={toggleTheme}
        >
          <Icon name={theme.name === "dark" ? "sun" : "moon"} size={20} color={theme.colors.text} />
        </IconButton>
        <IconButton label="Avisos" onPress={() => router.push("/(tabs)/avisos")} badge={unreadCount}>
          <Icon name="bell" size={20} color={theme.colors.text} />
        </IconButton>
      </View>
    </View>
  );
}

interface IconButtonProps {
  label: string;
  onPress: () => void;
  badge?: number;
  children: ReactElement;
}

function IconButton({ label, onPress, badge = 0, children }: IconButtonProps): ReactElement {
  const theme = useTheme();
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      onPress={onPress}
      style={({ pressed }) => [
        styles.iconBtn,
        { backgroundColor: theme.colors.surface2, transform: [{ scale: pressed ? 0.92 : 1 }] },
      ]}
    >
      {children}
      {badge > 0 ? (
        <View style={[styles.badge, { backgroundColor: theme.palette.red, borderColor: theme.colors.bg }]}>
          <Text style={styles.badgeText}>{badge}</Text>
        </View>
      ) : null}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  header: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    paddingHorizontal: 16,
    paddingBottom: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  id: { flexDirection: "row", alignItems: "center", gap: 11, flexShrink: 1, minWidth: 0 },
  idText: { minWidth: 0, flexShrink: 1 },
  greet: { fontSize: 12, fontWeight: "700" },
  name: { fontSize: 20, fontWeight: "800", letterSpacing: -0.4 },
  actions: { marginLeft: "auto", flexDirection: "row", alignItems: "center", gap: 8 },
  iconBtn: { width: 42, height: 42, borderRadius: 21, alignItems: "center", justifyContent: "center" },
  badge: {
    position: "absolute",
    top: -3,
    right: -3,
    minWidth: 18,
    height: 18,
    paddingHorizontal: 5,
    borderRadius: 9,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 2.5,
  },
  badgeText: { color: "#FFFFFF", fontSize: 11, fontWeight: "800" },
});

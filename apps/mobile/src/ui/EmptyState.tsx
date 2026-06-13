import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { Icon, type IconName } from "../icons";
import { useTheme } from "../theme";

export interface EmptyStateProps {
  icon: IconName;
  title: string;
  subtitle: string;
}

/** Friendly empty state — rounded icon tile, title, supportive subtitle. */
export function EmptyState({ icon, title, subtitle }: EmptyStateProps): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.wrap}>
      <View style={[styles.iconTile, { backgroundColor: theme.colors.surfaceSunken }]}>
        <Icon name={icon} size={34} color={theme.colors.text3} />
      </View>
      <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        {title}
      </Text>
      <Text style={[styles.sub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
        {subtitle}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { alignItems: "center", paddingVertical: 46, paddingHorizontal: 24 },
  iconTile: { width: 76, height: 76, borderRadius: 26, marginBottom: 16, alignItems: "center", justifyContent: "center" },
  title: { fontSize: 18, fontWeight: "800" },
  sub: { fontSize: 13.5, fontWeight: "600", marginTop: 6, textAlign: "center" },
});

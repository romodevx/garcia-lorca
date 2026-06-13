import { router } from "expo-router";
import { type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { Icon } from "../icons";
import { useTheme } from "../theme";

export interface DetailHeaderProps {
  title: string;
}

/** Back-chevron header for pushed detail screens (aviso, módulo, eventos). */
export function DetailHeader({ title }: DetailHeaderProps): ReactElement {
  const theme = useTheme();
  const insets = useSafeAreaInsets();
  return (
    <View style={[styles.header, { paddingTop: insets.top + 12, backgroundColor: theme.colors.bg }]}>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Atrás"
        onPress={() => router.back()}
        style={({ pressed }) => [
          styles.iconBtn,
          { backgroundColor: theme.colors.surface2, transform: [{ scale: pressed ? 0.92 : 1 }] },
        ]}
      >
        <Icon name="chevron-left" size={22} color={theme.colors.text} />
      </Pressable>
      <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]} numberOfLines={1}>
        {title}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  header: { flexDirection: "row", alignItems: "center", gap: 12, paddingHorizontal: 16, paddingBottom: 10 },
  iconBtn: { width: 42, height: 42, borderRadius: 21, alignItems: "center", justifyContent: "center" },
  title: { fontSize: 19, fontWeight: "800", letterSpacing: -0.3, flexShrink: 1 },
});

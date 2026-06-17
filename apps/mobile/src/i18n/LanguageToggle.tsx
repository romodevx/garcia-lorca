import AsyncStorage from "@react-native-async-storage/async-storage";
import { type ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Pressable, StyleSheet, Text } from "react-native";
import { useTheme } from "../theme";
import { LOCALE_STORAGE_KEY } from "./config";

/**
 * Minimal es ⇆ en toggle for the parent app. Switches the live language and
 * persists the choice to AsyncStorage so the next launch restores it. Styled to
 * match the header's icon buttons. UX-only; not a full language menu.
 */
export function LanguageToggle(): ReactElement {
  const { t, i18n } = useTranslation("common");
  const theme = useTheme();
  const next = i18n.language === "es" ? "en" : "es";

  const onPress = (): void => {
    void i18n.changeLanguage(next);
    void AsyncStorage.setItem(LOCALE_STORAGE_KEY, next);
  };

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t("language.label")}
      onPress={onPress}
      style={({ pressed }) => [
        styles.btn,
        { backgroundColor: theme.colors.surface2, transform: [{ scale: pressed ? 0.92 : 1 }] },
      ]}
    >
      <Text style={[styles.label, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        {next.toUpperCase()}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  btn: { height: 42, minWidth: 42, paddingHorizontal: 12, borderRadius: 21, alignItems: "center", justifyContent: "center" },
  label: { fontSize: 13, fontWeight: "800" },
});

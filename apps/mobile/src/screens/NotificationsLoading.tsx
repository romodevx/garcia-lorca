import { type ReactElement } from "react";
import { ActivityIndicator, StyleSheet, View } from "react-native";
import { useTheme } from "../theme";

/**
 * Shared first-load spinner for the notifications inbox, rendered by both the list
 * and the single-notification detail so a cold-boot load looks the same in both.
 */
export function NotificationsLoading(): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.centered} testID="notifications-loading">
      <ActivityIndicator color={theme.colors.primary} />
    </View>
  );
}

const styles = StyleSheet.create({
  centered: { alignItems: "center" },
});

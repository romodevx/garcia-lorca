import { type ReactElement } from "react";
import { ActivityIndicator, StyleSheet, Text, View } from "react-native";
import { useTheme } from "../theme";
import { Button, EmptyState } from "../ui";
import type { IconName } from "../icons";

/**
 * Shared loading/error states for the network-backed family screens (account,
 * attendance, grades). Kept generic — the copy is passed in — so a failed load is
 * never mistaken for an empty result and every screen handles its four states the
 * same way.
 */

/** First-load spinner. Needs a testID so RNTL can assert the loading state. */
export function AsyncLoading(): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.centered} testID="async-loading">
      <ActivityIndicator color={theme.colors.primary} />
    </View>
  );
}

export interface AsyncErrorStateProps {
  title: string;
  subtitle: string;
  onRetry: () => void;
  icon?: IconName;
}

/** "We couldn't load this" state with a retry action. */
export function AsyncErrorState({ title, subtitle, onRetry, icon = "alert" }: AsyncErrorStateProps): ReactElement {
  return (
    <View style={styles.centered}>
      <EmptyState icon={icon} title={title} subtitle={subtitle} />
      <Button label="Reintentar" icon="refresh" variant="outline" small onPress={onRetry} style={styles.retry} />
    </View>
  );
}

/**
 * A background reload failed but the last good data is still on screen: say so
 * inline instead of replacing the content with a full-screen error.
 */
export function StaleNotice(): ReactElement {
  const theme = useTheme();
  return (
    <View
      style={[styles.notice, { backgroundColor: theme.colors.orangeSoft }]}
      accessibilityLiveRegion="polite"
      accessibilityRole="alert"
    >
      <Text style={[styles.noticeText, { color: theme.colors.tintOrangeInk, fontFamily: theme.fonts.uiSemi }]}>
        No pudimos actualizar. Desliza para reintentar.
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  notice: { borderRadius: 14, paddingHorizontal: 14, paddingVertical: 10, marginTop: 12 },
  noticeText: { fontSize: 14, fontWeight: "600" },
  centered: { alignItems: "center" },
  retry: { width: "auto", paddingHorizontal: 22, marginTop: -8 },
});

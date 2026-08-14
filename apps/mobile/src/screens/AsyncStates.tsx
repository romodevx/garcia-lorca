import { type ReactElement } from "react";
import { ActivityIndicator, StyleSheet, View } from "react-native";
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

const styles = StyleSheet.create({
  centered: { alignItems: "center" },
  retry: { width: "auto", paddingHorizontal: 22, marginTop: -8 },
});

import { type ReactElement } from "react";
import { StyleSheet, View } from "react-native";
import { Button, EmptyState } from "../ui";

/**
 * Shared "we couldn't load the inbox" state with a retry action, rendered by both
 * the notifications list and the single-notification detail so a failed load is
 * never mistaken for "no news" or a retracted notice.
 */
export function NotificationsErrorState({ onRetry }: { onRetry: () => void }): ReactElement {
  return (
    <View style={styles.centered}>
      <EmptyState
        icon="inbox"
        title="No pudimos cargar los avisos"
        subtitle="Revisa tu conexión e inténtalo de nuevo."
      />
      <Button label="Reintentar" icon="refresh" variant="outline" small onPress={onRetry} style={styles.retry} />
    </View>
  );
}

const styles = StyleSheet.create({
  centered: { alignItems: "center" },
  retry: { width: "auto", paddingHorizontal: 22, marginTop: -8 },
});

import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { Icon } from "../icons";
import { useAppState } from "../state/AppState";
import { useTheme } from "../theme";
import { Card, Chip, EmptyState, TintIcon } from "../ui";
import { notificationKind } from "./notificationView";

export interface AnnouncementDetailScreenProps {
  /** The notification id (string) from the family inbox. */
  id: string;
}

/**
 * Full notification view: kind chip, title, sender/date, body. It reads the
 * notification from the already-loaded family inbox in AppState (there is no
 * single-notification family endpoint), so a deep link that outlives the loaded
 * list falls back to a friendly empty state.
 */
export function AnnouncementDetailScreen({ id }: AnnouncementDetailScreenProps): ReactElement {
  const theme = useTheme();
  const { notifications } = useAppState();
  const notification = notifications.find((n) => n.id === id);

  if (!notification) {
    return <EmptyState icon="inbox" title="Aviso no disponible" subtitle="Es posible que se haya retirado." />;
  }

  const kind = notificationKind(notification.type);

  return (
    <>
      <View style={styles.head}>
        <TintIcon name={kind.icon} tint={kind.tint} size={46} icon={24} />
        <Chip kind={kind.chip}>{kind.label}</Chip>
      </View>

      <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>
        {notification.title}
      </Text>

      <View style={styles.meta}>
        <Icon name="user" size={15} color={theme.colors.text2} />
        <Text style={[styles.from, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>{kind.label}</Text>
        <Text style={[styles.sep, { color: theme.colors.text3 }]}>·</Text>
        <Text style={[styles.time, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
          {formatWhen(notification.createdAt)}
        </Text>
      </View>

      <Card padded style={{ marginTop: 16 }}>
        <Text style={[styles.body, { color: theme.colors.text2, fontFamily: theme.fonts.uiRegular }]}>
          {notification.body}
        </Text>
      </Card>
    </>
  );
}

/** Long es-MX date label from an RFC3339 instant; empty on an unparseable date. */
function formatWhen(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toLocaleDateString("es-MX", { day: "numeric", month: "long" });
}

const styles = StyleSheet.create({
  head: { flexDirection: "row", alignItems: "center", gap: 10, marginBottom: 14 },
  title: { fontSize: 24, fontWeight: "700", lineHeight: 28, letterSpacing: -0.4 },
  meta: { flexDirection: "row", alignItems: "center", gap: 7, marginTop: 10 },
  from: { fontSize: 13, fontWeight: "700" },
  sep: { fontSize: 13 },
  time: { fontSize: 13, fontWeight: "600" },
  body: { fontSize: 14.5, lineHeight: 23, fontWeight: "500" },
});

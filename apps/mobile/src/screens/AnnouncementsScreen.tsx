import { router } from "expo-router";
import { type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import type { FamilyNotification } from "../api";
import { useAppState } from "../state/AppState";
import { useTheme } from "../theme";
import { Card, EmptyState, ScreenTitle, TintIcon } from "../ui";
import { NotificationsErrorState } from "./NotificationsErrorState";
import { NotificationsLoading } from "./NotificationsLoading";
import { formatNotificationDate, isUnread, notificationKind } from "./notificationView";

/**
 * Avisos: the guardian's real notification inbox, loaded from
 * `GET /api/v1/family/notifications` via AppState. It handles the four states a
 * network-backed list must: loading, error (with retry), empty, and populated.
 *
 * "Marcar leídos" and tapping a row PERSIST the acknowledgement: markRead calls the
 * guardian-reachable `POST /api/v1/family/notifications/{id}/read` route, so the
 * server stamps `readAt` and the read state survives a relaunch (see
 * notificationView.isUnread). `readIds` is only an optimistic bridge that reflects
 * the acknowledgement immediately until the next refetch returns the stamped list.
 */
export function AnnouncementsScreen(): ReactElement {
  const {
    notifications,
    notificationsLoading,
    notificationsError,
    refreshNotifications,
    readIds,
    unreadCount,
    markRead,
    markAllRead,
  } = useAppState();

  const open = (id: string): void => {
    markRead(id);
    router.push({ pathname: "/aviso/[id]", params: { id } });
  };

  return (
    <>
      <View style={styles.titleRow}>
        <ScreenTitle>Avisos</ScreenTitle>
        {unreadCount > 0 ? <MarkAllAction onPress={markAllRead} /> : null}
      </View>

      <AnnouncementsBody
        notifications={notifications}
        loading={notificationsLoading}
        error={notificationsError}
        readIds={readIds}
        onRetry={refreshNotifications}
        onOpen={open}
      />
    </>
  );
}

function MarkAllAction({ onPress }: { onPress: () => void }): ReactElement {
  const theme = useTheme();
  return (
    <Pressable accessibilityRole="button" accessibilityLabel="Marcar leídos" onPress={onPress}>
      <Text style={[styles.action, { color: theme.colors.primaryInk, fontFamily: theme.fonts.uiExtra }]}>
        Marcar leídos
      </Text>
    </Pressable>
  );
}

interface BodyProps {
  notifications: FamilyNotification[];
  loading: boolean;
  error: boolean;
  readIds: ReadonlySet<string>;
  onRetry: () => void;
  onOpen: (id: string) => void;
}

/** Renders exactly one of: loading, error+retry, empty, or the notification list. */
function AnnouncementsBody({ notifications, loading, error, readIds, onRetry, onOpen }: BodyProps): ReactElement {
  // Error takes precedence over a stale/empty list so a failed refresh is never
  // mistaken for "no news".
  if (error) {
    return <NotificationsErrorState onRetry={onRetry} />;
  }

  // Only show the spinner on the FIRST load (nothing to show yet); a refresh over
  // existing content keeps the list visible.
  if (loading && notifications.length === 0) {
    return <NotificationsLoading />;
  }

  if (notifications.length === 0) {
    return (
      <EmptyState
        icon="inbox"
        title="No hay avisos nuevos"
        subtitle="Te avisaremos cuando la escuela publique algo."
      />
    );
  }

  return (
    <Card>
      {notifications.map((n, i) => (
        <NotificationRow
          key={n.id}
          notification={n}
          unread={isUnread(n, readIds)}
          divider={i > 0}
          onPress={() => onOpen(n.id)}
        />
      ))}
    </Card>
  );
}

interface RowProps {
  notification: FamilyNotification;
  unread: boolean;
  divider: boolean;
  onPress: () => void;
}

function NotificationRow({ notification, unread, divider, onPress }: RowProps): ReactElement {
  const theme = useTheme();
  const kind = notificationKind(notification.type);
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={notification.title}
      onPress={onPress}
      style={[
        styles.row,
        divider ? { borderTopColor: theme.colors.line, borderTopWidth: StyleSheet.hairlineWidth } : null,
      ]}
      android_ripple={{ color: theme.colors.surface2 }}
    >
      <View style={[styles.unreadDot, { backgroundColor: unread ? theme.colors.primary : "transparent" }]} />
      <TintIcon name={kind.icon} tint={kind.tint} size={42} />
      <View style={{ flex: 1, minWidth: 0 }}>
        <Text
          style={[
            styles.title,
            {
              color: unread ? theme.colors.text : theme.colors.text2,
              fontFamily: unread ? theme.fonts.uiExtra : theme.fonts.uiSemi,
            },
          ]}
        >
          {notification.title}
        </Text>
        <View style={styles.meta}>
          <Text style={[styles.from, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>{kind.label}</Text>
          <Text style={[styles.metaSep, { color: theme.colors.text3 }]}>·</Text>
          <Text style={[styles.time, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
            {formatNotificationDate(notification.createdAt, "short")}
          </Text>
        </View>
        <Text numberOfLines={2} style={[styles.excerpt, { color: theme.colors.text2, fontFamily: theme.fonts.uiRegular }]}>
          {notification.body}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  titleRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingTop: 8, paddingBottom: 12 },
  action: { fontSize: 13.5, fontWeight: "800" },
  row: { flexDirection: "row", alignItems: "flex-start", gap: 13, paddingVertical: 15, paddingHorizontal: 16 },
  unreadDot: { width: 9, height: 9, borderRadius: 4.5, marginTop: 7 },
  title: { fontSize: 15, lineHeight: 19 },
  meta: { flexDirection: "row", alignItems: "center", gap: 7, marginTop: 4, flexWrap: "wrap" },
  from: { fontSize: 12, fontWeight: "700" },
  metaSep: { fontSize: 12 },
  time: { fontSize: 12, fontWeight: "600" },
  excerpt: { fontSize: 13, fontWeight: "500", marginTop: 5, lineHeight: 18 },
});

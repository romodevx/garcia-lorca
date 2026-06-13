import { router } from "expo-router";
import { type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { announcements, catMeta } from "../data/mock";
import { useAppState } from "../state/AppState";
import { useTheme } from "../theme";
import { Card, EmptyState, ScreenTitle, TintIcon } from "../ui";

/**
 * Avisos: read/unread list with an unread dot, a "Marcar leídos" action that
 * clears every badge, and a friendly empty state when nothing is left. Tapping
 * a row marks it read and opens the detail.
 *
 * The list is non-empty by design (mock data), so the empty state is reached by
 * filtering; an `emptyOverride` prop lets tests assert the empty branch.
 */
export interface AnnouncementsScreenProps {
  /** Forces the empty state (used to verify the empty-state UI). */
  emptyOverride?: boolean;
}

export function AnnouncementsScreen({ emptyOverride = false }: AnnouncementsScreenProps): ReactElement {
  const theme = useTheme();
  const { readIds, unreadCount, markRead, markAllRead } = useAppState();
  const list = emptyOverride ? [] : announcements;

  const open = (id: number): void => {
    markRead(id);
    router.push({ pathname: "/aviso/[id]", params: { id: String(id) } });
  };

  return (
    <>
      <View style={styles.titleRow}>
        <ScreenTitle>Avisos</ScreenTitle>
        {unreadCount > 0 ? (
          <Pressable accessibilityRole="button" accessibilityLabel="Marcar leídos" onPress={markAllRead}>
            <Text style={[styles.action, { color: theme.colors.primaryInk, fontFamily: theme.fonts.uiExtra }]}>
              Marcar leídos
            </Text>
          </Pressable>
        ) : null}
      </View>

      {list.length === 0 ? (
        <EmptyState
          icon="inbox"
          title="No hay avisos nuevos"
          subtitle="Te avisaremos cuando la escuela publique algo."
        />
      ) : (
        <Card>
          {list.map((a, i) => {
            const isUnread = a.unread && !readIds.has(a.id);
            const cat = catMeta[a.cat];
            return (
              <Pressable
                key={a.id}
                accessibilityRole="button"
                accessibilityLabel={a.title}
                onPress={() => open(a.id)}
                style={[styles.row, i > 0 ? { borderTopColor: theme.colors.line, borderTopWidth: StyleSheet.hairlineWidth } : null]}
                android_ripple={{ color: theme.colors.surface2 }}
              >
                <View
                  style={[
                    styles.unreadDot,
                    { backgroundColor: isUnread ? theme.colors.primary : "transparent" },
                  ]}
                />
                <TintIcon name={cat.icon} tint={cat.tint} size={42} />
                <View style={{ flex: 1, minWidth: 0 }}>
                  <Text
                    style={[
                      styles.title,
                      {
                        color: isUnread ? theme.colors.text : theme.colors.text2,
                        fontFamily: isUnread ? theme.fonts.uiExtra : theme.fonts.uiSemi,
                      },
                    ]}
                  >
                    {a.title}
                  </Text>
                  <View style={styles.meta}>
                    <Text style={[styles.from, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>{a.from}</Text>
                    <Text style={[styles.metaSep, { color: theme.colors.text3 }]}>·</Text>
                    <Text style={[styles.time, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{a.time}</Text>
                  </View>
                  <Text style={[styles.excerpt, { color: theme.colors.text2, fontFamily: theme.fonts.uiRegular }]}>
                    {a.excerpt}
                  </Text>
                </View>
              </Pressable>
            );
          })}
        </Card>
      )}
    </>
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

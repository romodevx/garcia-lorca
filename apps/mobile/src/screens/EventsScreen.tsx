import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { events } from "../data/mock";
import { tintColors, useTheme } from "../theme";
import { Card, ListRow } from "../ui";

/** Eventos: the full agenda list (reached from the Inicio "próximo evento" tile). */
export function EventsScreen(): ReactElement {
  const theme = useTheme();
  return (
    <Card>
      {events.map((ev, i) => {
        const { bg, fg } = tintColors(theme.colors, ev.tint);
        return (
          <ListRow key={ev.title} divider={i > 0}>
            <View style={[styles.badge, { backgroundColor: bg }]}>
              <Text style={[styles.day, { color: fg, fontFamily: theme.fonts.display }]}>{ev.day}</Text>
              <Text style={[styles.mon, { color: fg }]}>{ev.mon}</Text>
            </View>
            <View style={{ flex: 1 }}>
              <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{ev.title}</Text>
              <Text style={[styles.sub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{ev.sub}</Text>
            </View>
          </ListRow>
        );
      })}
    </Card>
  );
}

const styles = StyleSheet.create({
  badge: { width: 50, height: 50, borderRadius: 15, alignItems: "center", justifyContent: "center" },
  day: { fontSize: 19, fontWeight: "700", lineHeight: 21 },
  mon: { fontSize: 9, fontWeight: "800", opacity: 0.8, marginTop: 1 },
  title: { fontSize: 16, fontWeight: "800" },
  sub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
});

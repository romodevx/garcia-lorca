import { useState, type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import type { AttendanceDay } from "../../api/attendance";
import { Icon } from "../../icons";
import { useTheme } from "../../theme";
import { Card, Chip } from "../../ui";
import { dayLabel, statusPill } from "../attendanceView";
import { ClassList } from "./TodayCard";

/** The month's school days, newest first. A per-class day opens to its classes. */
export function DayList({ days }: { days: AttendanceDay[] }): ReactElement {
  const theme = useTheme();
  if (days.length === 0) {
    return (
      <Card padded style={styles.card}>
        <Text style={[styles.empty, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>
          Sin días registrados en este mes
        </Text>
      </Card>
    );
  }
  return (
    <Card style={styles.card}>
      {days.map((d, i) => (
        <DayRow key={d.date} day={d} divider={i > 0} />
      ))}
    </Card>
  );
}

function DayRow({ day, divider }: { day: AttendanceDay; divider: boolean }): ReactElement {
  const theme = useTheme();
  const [open, setOpen] = useState(false);
  const pill = statusPill(day.status);
  const label = dayLabel(day.date);
  const expandable = day.classes.length > 0;

  const summary = (
    <View style={styles.row}>
      <View style={styles.main}>
        <Text style={[styles.date, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{label}</Text>
        {day.note ? (
          <Text style={[styles.note, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{day.note}</Text>
        ) : null}
      </View>
      <Chip kind={pill.kind}>{pill.label}</Chip>
      {expandable ? (
        <Icon name={open ? "chevron-down" : "chevron-right"} size={20} stroke={2.4} color={theme.colors.text3} />
      ) : null}
    </View>
  );

  return (
    <View style={[styles.item, divider && { borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.line }]}>
      {expandable ? (
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={`${label}: ${pill.label}. ${open ? "Ocultar clases" : "Ver clases"}`}
          accessibilityState={{ expanded: open }}
          onPress={() => setOpen((v) => !v)}
        >
          {summary}
        </Pressable>
      ) : (
        <View accessible accessibilityLabel={`${label}: ${pill.label}`}>
          {summary}
        </View>
      )}
      {expandable && open ? <ClassList classes={day.classes} /> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { marginTop: 8 },
  item: { paddingHorizontal: 16, paddingVertical: 12 },
  row: { flexDirection: "row", alignItems: "center", gap: 10, minHeight: 44 },
  main: { flex: 1 },
  date: { fontSize: 16, fontWeight: "800" },
  note: { fontSize: 14, fontWeight: "600", marginTop: 3 },
  empty: { fontSize: 16, fontWeight: "800", textAlign: "center" },
});

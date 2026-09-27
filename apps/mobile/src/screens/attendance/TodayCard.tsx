import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { AttendanceDay } from "../../api/attendance";
import { Icon } from "../../icons";
import { useTheme } from "../../theme";
import { Card, Chip } from "../../ui";
import { statusPill, todayHeadline } from "../attendanceView";
import { chipTint } from "./tint";

export interface TodayCardProps {
  /** Today's record, or null when today is unknown (not loaded, or no entry). */
  day: AttendanceDay | null;
}

/**
 * The first thing a parent reads: did my child make it to school today? A big
 * icon + words (never color alone); on a per-class day, each class with its own
 * status underneath.
 */
export function TodayCard({ day }: TodayCardProps): ReactElement {
  const theme = useTheme();
  const headline = todayHeadline(day);
  const tint = chipTint(theme.colors, headline.kind);
  const spoken = day?.note ? `Hoy: ${headline.spoken}. ${day.note}` : `Hoy: ${headline.spoken}`;
  return (
    <Card padded style={styles.card}>
      <View style={styles.top} accessible accessibilityLabel={spoken}>
        <View style={[styles.circle, { backgroundColor: tint.bg }]}>
          <Icon name={headline.icon} size={30} stroke={2.6} color={tint.fg} />
        </View>
        <View style={styles.main}>
          <Text style={[styles.label, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>Hoy</Text>
          <Text style={[styles.value, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{headline.text}</Text>
          {day?.note ? (
            <Text style={[styles.note, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{day.note}</Text>
          ) : null}
        </View>
      </View>
      {day && day.classes.length > 0 ? <ClassList classes={day.classes} /> : null}
    </Card>
  );
}

/** Each class of a per-class day with its status pill. Reused by the day list. */
export function ClassList({ classes }: { classes: AttendanceDay["classes"] }): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.classes}>
      {classes.map((c, i) => {
        const pill = statusPill(c.status);
        return (
          <View key={`${c.subjectName}-${i}`} style={styles.classRow} accessible accessibilityLabel={`${c.subjectName}: ${pill.label}`}>
            <View style={styles.main}>
              <Text style={[styles.subject, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{c.subjectName}</Text>
              {c.note ? (
                <Text style={[styles.note, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{c.note}</Text>
              ) : null}
            </View>
            <Chip kind={pill.kind}>{pill.label}</Chip>
          </View>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { marginTop: 12 },
  top: { flexDirection: "row", alignItems: "center", gap: 14 },
  circle: { width: 60, height: 60, borderRadius: 30, alignItems: "center", justifyContent: "center" },
  main: { flex: 1 },
  label: { fontSize: 14, fontWeight: "700" },
  value: { fontSize: 24, fontWeight: "800", marginTop: 2 },
  note: { fontSize: 14, fontWeight: "600", marginTop: 3 },
  classes: { marginTop: 14, gap: 10 },
  classRow: { flexDirection: "row", alignItems: "center", gap: 10, minHeight: 44 },
  subject: { fontSize: 16, fontWeight: "800" },
});

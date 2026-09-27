import { type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import type { AttendanceSummary } from "../../api/attendance";
import { Icon } from "../../icons";
import { useTheme } from "../../theme";
import { Card } from "../../ui";
import { monthLabel } from "../attendanceView";

export interface MonthSelectorProps {
  month: string;
  canGoNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
}

/** ‹ Septiembre 2026 › — never past the school's current month. */
export function MonthSelector({ month, canGoNext, onPrevious, onNext }: MonthSelectorProps): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.selector}>
      <Pressable accessibilityRole="button" accessibilityLabel="Mes anterior" onPress={onPrevious} style={styles.arrow} hitSlop={8}>
        <Icon name="chevron-left" size={24} stroke={2.6} color={theme.colors.text} />
      </Pressable>
      <Text accessibilityRole="header" style={[styles.month, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        {monthLabel(month)}
      </Text>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Mes siguiente"
        accessibilityState={{ disabled: !canGoNext }}
        disabled={!canGoNext}
        onPress={onNext}
        style={[styles.arrow, !canGoNext && styles.off]}
        hitSlop={8}
      >
        <Icon name="chevron-right" size={24} stroke={2.6} color={theme.colors.text} />
      </Pressable>
    </View>
  );
}

const COUNTS: { key: keyof AttendanceSummary; label: string }[] = [
  { key: "absent", label: "Faltas" },
  { key: "late", label: "Retardos" },
  { key: "excused", label: "Justificadas" },
  { key: "partial", label: "Días parciales" },
];

/** The month's exception counts, each a number with its word. */
export function MonthSummary({ summary }: { summary: AttendanceSummary }): ReactElement {
  const theme = useTheme();
  return (
    <Card style={styles.summary}>
      <View style={styles.grid}>
        {COUNTS.map(({ key, label }) => (
          <View key={key} style={styles.count} accessible accessibilityLabel={`${label}: ${summary[key]}`}>
            <Text style={[styles.num, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>{String(summary[key])}</Text>
            <Text style={[styles.lab, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>{label}</Text>
          </View>
        ))}
      </View>
    </Card>
  );
}

const styles = StyleSheet.create({
  selector: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginTop: 22 },
  arrow: { width: 44, height: 44, alignItems: "center", justifyContent: "center" },
  off: { opacity: 0.3 },
  month: { fontSize: 18, fontWeight: "800" },
  summary: { marginTop: 8 },
  grid: { flexDirection: "row", flexWrap: "wrap" },
  count: { width: "50%", alignItems: "center", paddingVertical: 12 },
  num: { fontSize: 26, fontWeight: "800" },
  lab: { fontSize: 14, fontWeight: "700", marginTop: 4 },
});

import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { EvaluationPeriod, GradeScale, PeriodSubject } from "../api/grades";
import { formatAverage, formatGrade, gradeLabel, isFailing, subjectAverageOf } from "../screens/gradesView";
import { useTheme } from "../theme";
import { BottomSheet, Button } from "../ui";

export interface SubjectSheetProps {
  visible: boolean;
  subject: PeriodSubject | null;
  /** Every evaluation period of the grade, by ordinal. */
  periods: EvaluationPeriod[];
  scale: GradeScale | null;
  onClose: () => void;
}

const MISSING = "—";
const FAILED = "Reprobada";

/**
 * One subject across the whole cycle: its average on top (the exact mean of its
 * period grades, truncated to 2 decimals), then each period with its grade ("—"
 * when not captured). A failing grade says "Reprobada" in words as well as in
 * colour.
 */
export function SubjectSheet({ visible, subject, periods, scale, onClose }: SubjectSheetProps): ReactElement {
  const theme = useTheme();
  if (subject === null) {
    return <BottomSheet visible={visible} onClose={onClose} />;
  }
  const name = subject.subjectName || "Materia sin nombre";
  const average = subjectAverageOf(subject);
  const averageText = average === undefined ? MISSING : formatAverage(average);

  return (
    <BottomSheet visible={visible} onClose={onClose}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        {name}
      </Text>
      <View
        accessible
        accessibilityLabel={`Promedio del ciclo: ${average === undefined ? "sin calificaciones aún" : averageText}`}
        style={styles.averageRow}
      >
        <Text style={[styles.averageLabel, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>Promedio del ciclo</Text>
        <Text style={[styles.averageValue, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{averageText}</Text>
      </View>

      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderColor: theme.colors.line }]}>
        {periods.map((period, i) => {
          const value = subject.grades[period.id];
          const failed = value !== undefined && isFailing(value, scale);
          return (
            <View
              key={period.id}
              accessible
              accessibilityLabel={gradeLabel(name, period.name, value, scale)}
              style={[styles.row, i > 0 ? { borderTopColor: theme.colors.line, borderTopWidth: StyleSheet.hairlineWidth } : null]}
            >
              <View style={styles.periodText}>
                <Text style={[styles.period, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{period.name}</Text>
                {failed ? <Text style={[styles.failed, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>{FAILED}</Text> : null}
              </View>
              <Text
                style={[
                  styles.grade,
                  { color: failed ? theme.colors.bad : value === undefined ? theme.colors.text3 : theme.colors.text, fontFamily: theme.fonts.uiExtra },
                ]}
              >
                {value === undefined ? MISSING : formatGrade(value)}
              </Text>
            </View>
          );
        })}
      </View>

      <Button label="Cerrar" variant="ghost" onPress={onClose} style={styles.close} />
    </BottomSheet>
  );
}

const MIN_TOUCH_TARGET = 44;

const styles = StyleSheet.create({
  title: { fontSize: 21, fontWeight: "800" },
  averageRow: { flexDirection: "row", alignItems: "baseline", justifyContent: "space-between", marginTop: 8, marginBottom: 12 },
  averageLabel: { fontSize: 16, fontWeight: "600" },
  averageValue: { fontSize: 28, fontWeight: "800" },
  card: { borderWidth: 1, borderRadius: 20, overflow: "hidden" },
  row: { flexDirection: "row", alignItems: "center", gap: 12, minHeight: MIN_TOUCH_TARGET + 8, paddingVertical: 12, paddingHorizontal: 16 },
  periodText: { flex: 1 },
  period: { fontSize: 17, fontWeight: "800" },
  failed: { fontSize: 15, fontWeight: "800", marginTop: 2 },
  grade: { fontSize: 22, fontWeight: "800" },
  close: { marginTop: 18 },
});

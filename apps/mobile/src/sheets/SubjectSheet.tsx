import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { GradeScale, MonthlySubject } from "../api/grades";
import { monthLabel } from "../screens/attendanceView";
import { formatAverage, formatGrade, gradeLabel, isFailing, subjectAverage } from "../screens/gradesView";
import { useTheme } from "../theme";
import { BottomSheet, Button } from "../ui";

export interface SubjectSheetProps {
  visible: boolean;
  subject: MonthlySubject | null;
  /** Every month of the cycle, oldest first. */
  months: string[];
  scale: GradeScale | null;
  onClose: () => void;
}

const MISSING = "—";
const FAILED = "Reprobada";

/**
 * One subject across the whole cycle: its average on top, then each month with
 * its grade ("—" when not captured). A failing grade says "Reprobada" in words
 * as well as in colour.
 */
export function SubjectSheet({ visible, subject, months, scale, onClose }: SubjectSheetProps): ReactElement {
  const theme = useTheme();
  if (subject === null) {
    return <BottomSheet visible={visible} onClose={onClose} />;
  }
  const name = subject.subjectName || "Materia sin nombre";
  const average = subjectAverage(subject);
  const averageText = average === null ? MISSING : formatAverage(average);

  return (
    <BottomSheet visible={visible} onClose={onClose}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        {name}
      </Text>
      <View
        accessible
        accessibilityLabel={`Promedio del ciclo: ${average === null ? "sin calificaciones aún" : averageText}`}
        style={styles.averageRow}
      >
        <Text style={[styles.averageLabel, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>Promedio del ciclo</Text>
        <Text style={[styles.averageValue, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{averageText}</Text>
      </View>

      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderColor: theme.colors.line }]}>
        {months.map((month, i) => {
          const value = subject.grades[month];
          const failed = value !== undefined && isFailing(value, scale);
          return (
            <View
              key={month}
              accessible
              accessibilityLabel={gradeLabel(name, month, value, scale)}
              style={[styles.row, i > 0 ? { borderTopColor: theme.colors.line, borderTopWidth: StyleSheet.hairlineWidth } : null]}
            >
              <View style={styles.monthText}>
                <Text style={[styles.month, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{monthLabel(month)}</Text>
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
  monthText: { flex: 1 },
  month: { fontSize: 17, fontWeight: "800" },
  failed: { fontSize: 15, fontWeight: "800", marginTop: 2 },
  grade: { fontSize: 22, fontWeight: "800" },
  close: { marginTop: 18 },
});

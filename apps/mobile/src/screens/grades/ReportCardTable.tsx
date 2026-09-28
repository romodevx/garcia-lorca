import { type ReactElement } from "react";
import { ScrollView, StyleSheet, Text, useWindowDimensions, View } from "react-native";
import type { ReportCard, ReportCardLine } from "../../api/reportCards";
import { useTheme } from "../../theme";
import { formatGrade, isFailing, monthName, shortMonthLabel } from "../gradesView";

const MISSING = "—";
/** Subject column width at the default text size; it fits "MATEMÁTICAS" on one line. */
const SUBJECT_COL_BASE_WIDTH = 128;
const FAILED = "Reprobada";

/** "Matemáticas: septiembre 9.5, octubre 8, noviembre sin calificación, promedio 8.75, reprobada". */
export function lineLabel(line: ReportCardLine, months: string[], card: Pick<ReportCard, "scale">): string {
  const perMonth = months.map((m) => {
    const value = line.months[m];
    return `${monthName(m)} ${value === undefined ? "sin calificación" : formatGrade(value)}`;
  });
  const average = line.average === null ? "promedio sin calificación" : `promedio ${formatGrade(line.average)}`;
  const failed = line.average !== null && isFailing(line.average, card.scale) ? ", reprobada" : "";
  return `${line.subjectName}: ${[...perMonth, average].join(", ")}${failed}`;
}

/** A row's React key: the subject id, or its position and name when the server sent none. */
export function lineKey(line: ReportCardLine, index: number): string {
  return line.subjectId || `${index}-${line.subjectName}`;
}

/**
 * The boleta as a table: one row per subject, one column per month, then the
 * term average. A failing average says "Reprobada" in words under the number.
 * Scrolls sideways when large text makes it wider than the phone. The subject
 * column grows with the system text size, so a one-word subject that fits at the
 * default size never splits mid-word ("MATEMÁTIC / AS") when the text is larger.
 */
export function ReportCardTable({ card }: { card: ReportCard }): ReactElement {
  const theme = useTheme();
  const { fontScale } = useWindowDimensions();
  const subjectCol = { width: SUBJECT_COL_BASE_WIDTH * Math.max(1, fontScale), textAlign: "left" as const };
  const head = [styles.head, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }];
  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.scroll}>
      <View style={[styles.table, { borderColor: theme.colors.line, backgroundColor: theme.colors.surface }]}>
        <View style={styles.row} importantForAccessibility="no-hide-descendants" accessibilityElementsHidden>
          <Text style={[head, subjectCol]}>Materia</Text>
          {card.months.map((m) => (
            <Text key={m} style={[head, styles.valueCol]}>{shortMonthLabel(m)}</Text>
          ))}
          <Text style={[head, styles.valueCol]}>Prom.</Text>
        </View>
        {card.lines.map((line, index) => {
          const failed = line.average !== null && isFailing(line.average, card.scale);
          const cell = [styles.cell, styles.valueCol, { color: theme.colors.text, fontFamily: theme.fonts.uiSemi }];
          return (
            <View
              key={lineKey(line, index)}
              accessible
              accessibilityLabel={lineLabel(line, card.months, card)}
              style={[styles.row, { borderTopColor: theme.colors.line, borderTopWidth: StyleSheet.hairlineWidth }]}
            >
              <Text style={[styles.cell, subjectCol, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
                {line.subjectName}
              </Text>
              {card.months.map((m) => (
                <Text key={m} style={cell}>{line.months[m] === undefined ? MISSING : formatGrade(line.months[m])}</Text>
              ))}
              <View style={styles.valueCol}>
                <Text style={[styles.cell, styles.average, { color: failed ? theme.colors.bad : theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
                  {line.average === null ? MISSING : formatGrade(line.average)}
                </Text>
                {failed ? <Text style={[styles.failed, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>{FAILED}</Text> : null}
              </View>
            </View>
          );
        })}
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  scroll: { flexGrow: 1 },
  table: { flexGrow: 1, borderWidth: 1, borderRadius: 20, overflow: "hidden", marginTop: 14 },
  row: { flexDirection: "row", alignItems: "center", minHeight: 48, paddingHorizontal: 12, paddingVertical: 8 },
  head: { fontSize: 14, fontWeight: "800", textAlign: "center" },
  valueCol: { minWidth: 44, flexGrow: 1, alignItems: "center", textAlign: "center" },
  cell: { fontSize: 17 },
  average: { fontWeight: "800", textAlign: "center" },
  failed: { fontSize: 12, fontWeight: "800", textAlign: "center" },
});

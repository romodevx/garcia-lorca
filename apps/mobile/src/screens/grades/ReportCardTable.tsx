import { type ReactElement } from "react";
import { ScrollView, StyleSheet, Text, useWindowDimensions, View } from "react-native";
import type { ReportCard, ReportCardLine } from "../../api/reportCards";
import { useTheme } from "../../theme";
import { formatAverage, formatGrade, isFailing } from "../gradesView";
import { reportCardColumns, type ReportCardColumn } from "./reportCardView";

const MISSING = "—";
/** Subject column width at the default text size; it fits "MATEMÁTICAS" on one line. */
const SUBJECT_COL_BASE_WIDTH = 128;
const FAILED = "Reprobada";

/** "Matemáticas: 1.er trimestre 9, 2.º trimestre 8, 3.er trimestre sin calificación, promedio 8.5, reprobada". */
export function lineLabel(line: ReportCardLine, columns: ReportCardColumn[], card: Pick<ReportCard, "scale">): string {
  const perColumn = columns.map((c) => {
    const value = c.valueOf(line);
    return `${c.spoken} ${value === undefined ? "sin calificación" : formatGrade(value)}`;
  });
  const average = line.average === null ? "promedio sin calificación" : `promedio ${formatAverage(line.average)}`;
  const failed = line.average !== null && isFailing(line.average, card.scale) ? ", reprobada" : "";
  return `${line.subjectName}: ${[...perColumn, average].join(", ")}${failed}`;
}

/** A row's React key: the subject id, or its position and name when the server sent none. */
export function lineKey(line: ReportCardLine, index: number): string {
  return line.subjectId || `${index}-${line.subjectName}`;
}

/**
 * The boleta as a table: one row per subject, one column per evaluation period
 * (per month on a card issued before T15), then the subject's average. A failing average says "Reprobada" in words under the number.
 * Scrolls sideways when large text makes it wider than the phone. The subject
 * column grows with the system text size, so a one-word subject that fits at the
 * default size never splits mid-word ("MATEMÁTIC / AS") when the text is larger.
 */
export function ReportCardTable({ card }: { card: ReportCard }): ReactElement {
  const theme = useTheme();
  const { fontScale } = useWindowDimensions();
  const subjectCol = { width: SUBJECT_COL_BASE_WIDTH * Math.max(1, fontScale), textAlign: "left" as const };
  const head = [styles.head, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }];
  const columns = reportCardColumns(card);
  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.scroll}>
      <View style={[styles.table, { borderColor: theme.colors.line, backgroundColor: theme.colors.surface }]}>
        <View style={styles.row} importantForAccessibility="no-hide-descendants" accessibilityElementsHidden>
          <Text style={[head, subjectCol]}>Materia</Text>
          {columns.map((c) => (
            <Text key={c.key} style={[head, styles.valueCol]}>{c.header}</Text>
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
              accessibilityLabel={lineLabel(line, columns, card)}
              style={[styles.row, { borderTopColor: theme.colors.line, borderTopWidth: StyleSheet.hairlineWidth }]}
            >
              <Text style={[styles.cell, subjectCol, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
                {line.subjectName}
              </Text>
              {columns.map((c) => {
                const value = c.valueOf(line);
                return <Text key={c.key} style={cell}>{value === undefined ? MISSING : formatGrade(value)}</Text>;
              })}
              <View style={styles.valueCol}>
                <Text style={[styles.cell, styles.average, { color: failed ? theme.colors.bad : theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
                  {line.average === null ? MISSING : formatAverage(line.average)}
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

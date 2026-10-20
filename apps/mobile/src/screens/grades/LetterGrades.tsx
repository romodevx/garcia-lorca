import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { EvaluationPeriod, PeriodSubject } from "../../api/grades";
import { useTheme } from "../../theme";
import { Card, ListRow, SectionLabel } from "../../ui";
import { letterMeaning, LETTERS_LEGEND } from "../gradesView";

const MISSING_SUBJECT_NAME = "Materia sin nombre";
const MISSING = "—";

export interface LetterGradesProps {
  subjects: PeriodSubject[];
  period: EvaluationPeriod;
}

/**
 * One period of a level that grades with letters (preescolar): each subject's
 * letter with what it means, and the legend on top. Letters are never
 * averaged and never "fail".
 */
export function LetterGrades({ subjects, period }: LetterGradesProps): ReactElement {
  const theme = useTheme();
  return (
    <>
      <Card style={styles.legend}>
        <Text style={[styles.legendText, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>{LETTERS_LEGEND}</Text>
      </Card>
      <SectionLabel>Materias</SectionLabel>
      <Card>
        {subjects.map((s, i) => {
          const name = s.subjectName || MISSING_SUBJECT_NAME;
          const letter = s.letters[period.id];
          const meaning = letter === undefined ? undefined : letterMeaning(letter);
          const spoken = letter === undefined ? "sin calificación aún" : [letter, meaning].filter(Boolean).join(", ");
          return (
            <ListRow key={s.subjectId} divider={i > 0}>
              {/* One element for a screen reader: the subject, the period and the letter's meaning. */}
              <View accessible accessibilityLabel={`${name}, ${period.name}: ${spoken}`} style={styles.subject}>
                <View style={styles.subjectText}>
                  <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{name}</Text>
                  {meaning === undefined ? null : (
                    <Text style={[styles.note, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>{meaning}</Text>
                  )}
                </View>
                <Text style={[styles.letter, { color: letter === undefined ? theme.colors.text3 : theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
                  {letter ?? MISSING}
                </Text>
              </View>
            </ListRow>
          );
        })}
      </Card>
    </>
  );
}

const styles = StyleSheet.create({
  legend: { padding: 16, marginTop: 14 },
  legendText: { fontSize: 15, fontWeight: "600", lineHeight: 21 },
  subject: { flex: 1, flexDirection: "row", alignItems: "center", gap: 12 },
  subjectText: { flex: 1 },
  name: { fontSize: 17, fontWeight: "800" },
  note: { fontSize: 15, fontWeight: "600", marginTop: 3 },
  letter: { fontSize: 26, fontWeight: "800" },
});

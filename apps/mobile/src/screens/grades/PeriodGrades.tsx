import { useState, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { EvaluationPeriod, GradeScale, PeriodSubject } from "../../api/grades";
import { Icon } from "../../icons";
import { useTheme } from "../../theme";
import { Card, ListRow, SectionLabel } from "../../ui";
import { SubjectSheet } from "../../sheets/SubjectSheet";
import { formatAverage, formatGrade, generalAverageOf, gradeLabel, isFailing, subjectAverageOf } from "../gradesView";

const MISSING_SUBJECT_NAME = "Materia sin nombre";
const MISSING = "—";
const FAILED = "Reprobada";

export interface PeriodGradesProps {
  subjects: PeriodSubject[];
  period: EvaluationPeriod;
  /** Every period of the grade, by ordinal — the subject sheet lists them all. */
  periods: EvaluationPeriod[];
  /** Null when there is nothing to judge by: no passing line, nothing marked failing. */
  scale: GradeScale | null;
}

/**
 * One period: the general average on top, then every subject with its grade
 * in the period ("—" when not captured) and its average across periods.
 * Tapping a subject opens its grade in every period.
 */
export function PeriodGrades({ subjects, period, periods, scale }: PeriodGradesProps): ReactElement {
  const [openSubjectId, setOpenSubjectId] = useState<string | null>(null);
  const openSubject = subjects.find((s) => s.subjectId === openSubjectId) ?? null;
  return (
    <>
      <AverageCard average={generalAverageOf(subjects)} scale={scale} />
      <SectionLabel>Materias</SectionLabel>
      <Card>
        {subjects.map((s, i) => {
          const name = s.subjectName || MISSING_SUBJECT_NAME;
          const average = subjectAverageOf(s);
          const spokenAverage = average === undefined ? "" : `. Promedio ${formatAverage(average)}`;
          return (
            <ListRow
              key={s.subjectId}
              divider={i > 0}
              onPress={() => setOpenSubjectId(s.subjectId)}
              accessibilityLabel={`${gradeLabel(name, period.name, s.grades[period.id], scale)}${spokenAverage}`}
              accessibilityHint="Ver todos los periodos"
            >
              <SubjectGrade name={name} value={s.grades[period.id]} average={average} scale={scale} />
            </ListRow>
          );
        })}
      </Card>
      <SubjectSheet
        visible={openSubject !== null}
        subject={openSubject}
        periods={periods}
        scale={scale}
        onClose={() => setOpenSubjectId(null)}
      />
    </>
  );
}

function AverageCard({ average, scale }: { average: number | undefined; scale: GradeScale | null }): ReactElement {
  const theme = useTheme();
  const text = average === undefined ? MISSING : formatAverage(average);
  return (
    <Card style={styles.hero}>
      <View accessible accessibilityLabel={`Promedio general: ${average === undefined ? "sin calificación aún" : text}`}>
        <Text style={[styles.heroLabel, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>Promedio general</Text>
        <Text style={[styles.heroNum, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{text}</Text>
      </View>
      {scale === null ? null : (
        <Text style={[styles.heroRule, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
          {`Aprobatoria: ${formatGrade(scale.passing)} de ${formatGrade(scale.max)}`}
        </Text>
      )}
    </Card>
  );
}

interface SubjectGradeProps {
  name: string;
  value: number | undefined;
  average: number | undefined;
  scale: GradeScale | null;
}

function SubjectGrade({ name, value, average, scale }: SubjectGradeProps): ReactElement {
  const theme = useTheme();
  const failed = value !== undefined && isFailing(value, scale);
  const gradeColor = failed ? theme.colors.bad : value === undefined ? theme.colors.text3 : theme.colors.text;
  return (
    <View style={styles.subject}>
      <View style={styles.subjectText}>
        <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{name}</Text>
        {average === undefined ? null : (
          <Text style={[styles.note, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
            {`Promedio ${formatAverage(average)}`}
          </Text>
        )}
        {failed ? <Text style={[styles.note, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>{FAILED}</Text> : null}
      </View>
      <Text style={[styles.grade, { color: gradeColor, fontFamily: theme.fonts.uiExtra }]}>
        {value === undefined ? MISSING : formatGrade(value)}
      </Text>
      <Icon name="chevron-right" size={20} color={theme.colors.text3} />
    </View>
  );
}

const styles = StyleSheet.create({
  hero: { padding: 20, marginTop: 14, gap: 6 },
  heroLabel: { fontSize: 15, fontWeight: "600" },
  heroNum: { fontSize: 34, fontWeight: "800" },
  heroRule: { fontSize: 14, fontWeight: "600" },
  subject: { flex: 1, flexDirection: "row", alignItems: "center", gap: 12 },
  subjectText: { flex: 1 },
  name: { fontSize: 17, fontWeight: "800" },
  note: { fontSize: 15, fontWeight: "600", marginTop: 3 },
  grade: { fontSize: 24, fontWeight: "800" },
});

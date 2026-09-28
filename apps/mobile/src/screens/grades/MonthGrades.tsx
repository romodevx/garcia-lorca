import { useState, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { GradeScale, MonthlySubject } from "../../api/grades";
import { Icon } from "../../icons";
import { useTheme } from "../../theme";
import { Card, ListRow, SectionLabel } from "../../ui";
import { SubjectSheet } from "../../sheets/SubjectSheet";
import { formatAverage, formatGrade, gradeLabel, isFailing, monthAverage } from "../gradesView";

const MISSING_SUBJECT_NAME = "Materia sin nombre";
const NO_GRADE = "Sin calificación aún";
const FAILED = "Reprobada";

export interface MonthGradesProps {
  subjects: MonthlySubject[];
  month: string;
  /** Every month of the cycle, oldest first — the subject sheet lists them all. */
  months: string[];
  /** Null when there is nothing to judge by: no passing line, nothing marked failing. */
  scale: GradeScale | null;
}

/**
 * One month: its average on top, then every subject with its grade or
 * "Sin calificación aún". Tapping a subject opens its grades for the whole cycle.
 */
export function MonthGrades({ subjects, month, months, scale }: MonthGradesProps): ReactElement {
  const [openSubjectId, setOpenSubjectId] = useState<string | null>(null);
  const openSubject = subjects.find((s) => s.subjectId === openSubjectId) ?? null;
  return (
    <>
      <AverageCard average={monthAverage(subjects, month)} scale={scale} />
      <SectionLabel>Materias</SectionLabel>
      <Card>
        {subjects.map((s, i) => {
          const name = s.subjectName || MISSING_SUBJECT_NAME;
          return (
            <ListRow
              key={s.subjectId}
              divider={i > 0}
              onPress={() => setOpenSubjectId(s.subjectId)}
              accessibilityLabel={gradeLabel(name, month, s.grades[month], scale)}
              accessibilityHint="Ver todos los meses"
            >
              <SubjectGrade name={name} value={s.grades[month]} scale={scale} />
            </ListRow>
          );
        })}
      </Card>
      <SubjectSheet
        visible={openSubject !== null}
        subject={openSubject}
        months={months}
        scale={scale}
        onClose={() => setOpenSubjectId(null)}
      />
    </>
  );
}

function AverageCard({ average, scale }: { average: number | null; scale: GradeScale | null }): ReactElement {
  const theme = useTheme();
  const text = average === null ? "—" : formatAverage(average);
  return (
    <Card style={styles.hero}>
      <View accessible accessibilityLabel={`Promedio del mes: ${average === null ? NO_GRADE : text}`}>
        <Text style={[styles.heroLabel, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>Promedio del mes</Text>
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

function SubjectGrade({ name, value, scale }: { name: string; value: number | undefined; scale: GradeScale | null }): ReactElement {
  const theme = useTheme();
  const failed = value !== undefined && isFailing(value, scale);
  const grade = value === undefined ? null : formatGrade(value);
  const gradeColor = failed ? theme.colors.bad : theme.colors.text;
  return (
    <View style={styles.subject}>
      <View style={styles.subjectText}>
        <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{name}</Text>
        {grade === null ? (
          <Text style={[styles.note, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>{NO_GRADE}</Text>
        ) : null}
        {failed ? <Text style={[styles.note, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>{FAILED}</Text> : null}
      </View>
      {grade === null ? null : (
        <Text style={[styles.grade, { color: gradeColor, fontFamily: theme.fonts.uiExtra }]}>{grade}</Text>
      )}
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

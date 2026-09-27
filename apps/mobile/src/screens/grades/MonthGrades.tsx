import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { GradeScale, MonthlySubject } from "../../api/grades";
import { useTheme } from "../../theme";
import { Card, ListRow, SectionLabel } from "../../ui";
import { formatGrade, isFailing, monthAverage } from "../gradesView";

const MISSING_SUBJECT_NAME = "Materia sin nombre";
const NO_GRADE = "Sin calificación aún";
const FAILED = "Reprobada";

export interface MonthGradesProps {
  subjects: MonthlySubject[];
  month: string;
  scale: GradeScale;
}

/** One month: its average on top, then every subject with its grade or "Sin calificación aún". */
export function MonthGrades({ subjects, month, scale }: MonthGradesProps): ReactElement {
  return (
    <>
      <AverageCard average={monthAverage(subjects, month)} scale={scale} />
      <SectionLabel>Materias</SectionLabel>
      <Card>
        {subjects.map((s, i) => (
          <ListRow key={s.subjectId} divider={i > 0}>
            <SubjectGrade name={s.subjectName || MISSING_SUBJECT_NAME} value={s.grades[month]} scale={scale} />
          </ListRow>
        ))}
      </Card>
    </>
  );
}

function AverageCard({ average, scale }: { average: number | null; scale: GradeScale }): ReactElement {
  const theme = useTheme();
  const text = average === null ? "—" : formatGrade(average, scale);
  return (
    <Card style={styles.hero}>
      <View accessible accessibilityLabel={`Promedio del mes: ${average === null ? NO_GRADE : text}`}>
        <Text style={[styles.heroLabel, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>Promedio del mes</Text>
        <Text style={[styles.heroNum, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{text}</Text>
      </View>
      <Text style={[styles.heroRule, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
        {`Aprobatoria: ${formatGrade(scale.passing, scale)} de ${formatGrade(scale.max, scale)}`}
      </Text>
    </Card>
  );
}

function SubjectGrade({ name, value, scale }: { name: string; value: number | undefined; scale: GradeScale }): ReactElement {
  const theme = useTheme();
  const failed = value !== undefined && isFailing(value, scale);
  const grade = value === undefined ? null : formatGrade(value, scale);
  const label = grade === null ? `${name}: ${NO_GRADE}` : `${name}: ${grade}${failed ? `, ${FAILED}` : ""}`;
  const gradeColor = failed ? theme.colors.bad : theme.colors.text;
  return (
    <View accessible accessibilityLabel={label} style={styles.subject}>
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

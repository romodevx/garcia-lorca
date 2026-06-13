import { useState, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import {
  gradesNum,
  gradesQual,
  qualMeta,
  type LevelId,
  type NumericSubject,
  type QualSubject,
} from "../data/mock";
import { SubjectSheet } from "../sheets/SubjectSheet";
import { useAppState } from "../state/AppState";
import { useTheme, type ThemeColors } from "../theme";
import { Card, Chip, ListRow, ProgressBar, Ring, ScreenTitle, SectionLabel, Segment, TintIcon } from "../ui";

function numericBadge(colors: ThemeColors, g: number): { bg: string; fg: string } {
  if (g >= 9) return { bg: colors.okBg, fg: colors.ok };
  if (g >= 8) return { bg: colors.primarySoft, fg: colors.primaryInk };
  if (g >= 7) return { bg: colors.warnBg, fg: colors.warn };
  return { bg: colors.badBg, fg: colors.bad };
}

function fmt(n: number): string {
  return n === 10 ? "10" : n.toFixed(1);
}

const LEVEL_OPTIONS: ReadonlyArray<{ value: LevelId; label: string }> = [
  { value: "kinder", label: "Kínder" },
  { value: "primaria", label: "Primaria" },
  { value: "secundaria", label: "Secundaria" },
];

/**
 * Calificaciones: a level toggle flips the whole screen between numeric (6–10,
 * with a ring-average and per-subject sheets) and qualitative (Logrado / En
 * proceso, with progress bars). The promedio is always visible for numeric
 * levels; qualitative shows the achieved/in-progress/needs-support summary.
 */
export function GradesScreen(): ReactElement {
  const theme = useTheme();
  const { level, levelId, setLevelId } = useAppState();
  const [subject, setSubject] = useState<NumericSubject | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const qual = level.grading === "qual";
  const period = qual ? gradesQual.period : gradesNum.period;

  const openSubject = (s: NumericSubject): void => {
    setSubject(s);
    setSheetOpen(true);
  };

  return (
    <>
      <View style={styles.titleRow}>
        <ScreenTitle>Calificaciones</ScreenTitle>
        <Chip kind="lime" icon="calendar">{period}</Chip>
      </View>

      {/* Level toggle — flips numeric ↔ cualitativo */}
      <View style={{ marginBottom: 4 }}>
        <Segment options={LEVEL_OPTIONS} value={levelId} onChange={setLevelId} />
      </View>

      {qual ? <QualHero /> : <NumericHero />}

      <SectionLabel>{qual ? "Campos formativos" : "Materias"}</SectionLabel>
      <Card>
        {qual
          ? gradesQual.subjects.map((s, i) => <QualRow key={s.name} subject={s} divider={i > 0} />)
          : gradesNum.subjects.map((s, i) => {
              const badge = numericBadge(theme.colors, s.grade);
              return (
                <ListRow key={s.name} divider={i > 0} onPress={() => openSubject(s)} accessibilityLabel={s.name}>
                  <TintIcon name={s.icon} tint={s.tint} />
                  <View style={{ flex: 1 }}>
                    <Text style={[styles.subjTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{s.name}</Text>
                    <Text style={[styles.subjSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
                      3 parciales · prom. {s.grade.toFixed(1)}
                    </Text>
                  </View>
                  <View style={[styles.gradeBadge, { backgroundColor: badge.bg }]}>
                    <Text style={[styles.gradeText, { color: badge.fg, fontFamily: theme.fonts.uiExtra }]}>{fmt(s.grade)}</Text>
                  </View>
                </ListRow>
              );
            })}
      </Card>
      {!qual ? (
        <Text style={[styles.hint, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>
          Toca una materia para ver los parciales
        </Text>
      ) : null}

      <SubjectSheet visible={sheetOpen} subject={subject} onClose={() => setSheetOpen(false)} />
    </>
  );
}

function NumericHero(): ReactElement {
  const theme = useTheme();
  return (
    <Card style={styles.avgHero}>
      <Ring pct={gradesNum.average / 10} color={theme.colors.ok} track={theme.colors.surfaceSunken}>
        <Text style={[styles.avgNum, { color: theme.colors.ok, fontFamily: theme.fonts.uiExtra }]}>
          {gradesNum.average.toFixed(1)}
        </Text>
        <Text style={[styles.avgOf, { color: theme.colors.text3, fontFamily: theme.fonts.ui }]}>de 10</Text>
      </Ring>
      <View style={{ flex: 1 }}>
        <Text style={[styles.heroTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Promedio general</Text>
        <Text style={[styles.heroSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
          {gradesNum.period} · {gradesNum.subjects.length} materias
        </Text>
        <View style={{ marginTop: 10 }}>
          <Chip kind="ok" icon="arrow-up-right">Muy bien</Chip>
        </View>
      </View>
    </Card>
  );
}

function QualHero(): ReactElement {
  const theme = useTheme();
  const items = [
    { key: "logrado", n: gradesQual.summary.logrado, color: theme.colors.ok },
    { key: "proceso", n: gradesQual.summary.proceso, color: theme.colors.warn },
    { key: "apoyo", n: gradesQual.summary.apoyo, color: theme.colors.bad },
  ] as const;
  return (
    <Card padded>
      <Text style={[styles.heroTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        Evaluación cualitativa
      </Text>
      <Text style={[styles.heroSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi, marginBottom: 14 }]}>
        {gradesQual.period} · {gradesQual.subjects.length} campos
      </Text>
      <View style={styles.qualGrid}>
        {items.map((it) => (
          <View key={it.key} style={[styles.qualStat, { backgroundColor: theme.colors.surface2 }]}>
            <Text style={[styles.statNum, { color: it.color, fontFamily: theme.fonts.display }]}>{it.n}</Text>
            <Text style={[styles.statLab, { color: theme.colors.text3, fontFamily: theme.fonts.ui }]}>
              {qualMeta[it.key].label}
            </Text>
          </View>
        ))}
      </View>
    </Card>
  );
}

function QualRow({ subject, divider }: { subject: QualSubject; divider: boolean }): ReactElement {
  const theme = useTheme();
  const meta = qualMeta[subject.level];
  const tint = subject.level === "logrado" ? "forest" : subject.level === "proceso" ? "orange" : "red";
  const barColor = meta.chip === "ok" ? theme.colors.ok : meta.chip === "warn" ? theme.colors.warn : theme.colors.bad;
  const badge = meta.chip === "ok"
    ? { bg: theme.colors.okBg, fg: theme.colors.ok }
    : meta.chip === "warn"
      ? { bg: theme.colors.warnBg, fg: theme.colors.warn }
      : { bg: theme.colors.badBg, fg: theme.colors.bad };
  return (
    <ListRow divider={divider}>
      <TintIcon name={subject.icon} tint={tint} />
      <View style={{ flex: 1 }}>
        <Text style={[styles.subjTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{subject.name}</Text>
        <ProgressBar pct={meta.pct / 100} color={barColor} style={{ marginTop: 8 }} />
      </View>
      <View style={[styles.qualBadge, { backgroundColor: badge.bg }]}>
        <Text style={[styles.qualBadgeText, { color: badge.fg, fontFamily: theme.fonts.uiExtra }]}>{meta.label}</Text>
      </View>
    </ListRow>
  );
}

const styles = StyleSheet.create({
  titleRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingTop: 8, paddingBottom: 12 },
  avgHero: { padding: 20, flexDirection: "row", alignItems: "center", gap: 18 },
  avgNum: { fontSize: 30, fontWeight: "800", letterSpacing: -0.5 },
  avgOf: { fontSize: 12, fontWeight: "700" },
  heroTitle: { fontSize: 19, fontWeight: "800" },
  heroSub: { fontSize: 13, fontWeight: "600", marginTop: 2 },
  qualGrid: { flexDirection: "row", gap: 12 },
  qualStat: { flex: 1, alignItems: "center", paddingVertical: 14, borderRadius: 16 },
  statNum: { fontSize: 26, fontWeight: "800", letterSpacing: -0.5 },
  statLab: { fontSize: 11.5, fontWeight: "700", marginTop: 6, textAlign: "center" },
  subjTitle: { fontSize: 15, fontWeight: "800" },
  subjSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
  gradeBadge: { minWidth: 46, height: 46, borderRadius: 23, paddingHorizontal: 8, alignItems: "center", justifyContent: "center" },
  gradeText: { fontSize: 20, fontWeight: "800" },
  qualBadge: { height: 30, paddingHorizontal: 12, borderRadius: 999, alignItems: "center", justifyContent: "center" },
  qualBadgeText: { fontSize: 12.5, fontWeight: "800" },
  hint: { fontSize: 12, fontWeight: "700", marginTop: 12, textAlign: "center" },
});

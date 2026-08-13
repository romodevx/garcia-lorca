import { useMemo, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyGradeEntry } from "../api";
import { useAppState } from "../state/AppState";
import { toChildView } from "../state/childView";
import { useChildGrades } from "../state/familyData";
import { useTheme, type ThemeColors } from "../theme";
import { Card, EmptyState, ListRow, Ring, ScreenTitle, SectionLabel } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";

/** One subject's entries collapsed into a display row. */
interface SubjectGroup {
  subjectName: string;
  average: number;
  termCount: number;
}

function numericBadge(colors: ThemeColors, g: number): { bg: string; fg: string } {
  if (g >= 9) return { bg: colors.okBg, fg: colors.ok };
  if (g >= 8) return { bg: colors.primarySoft, fg: colors.primaryInk };
  if (g >= 7) return { bg: colors.warnBg, fg: colors.warn };
  return { bg: colors.badBg, fg: colors.bad };
}

function fmt(n: number): string {
  return n === 10 ? "10" : n.toFixed(1);
}

function mean(values: number[]): number {
  return values.length === 0 ? 0 : values.reduce((a, b) => a + b, 0) / values.length;
}

/** Collapses the flat entries into one row per subject, preserving first-seen order. */
function groupBySubject(entries: FamilyGradeEntry[]): SubjectGroup[] {
  const order: string[] = [];
  const byName = new Map<string, number[]>();
  for (const e of entries) {
    const name = e.subjectName || "Materia";
    if (!byName.has(name)) {
      byName.set(name, []);
      order.push(name);
    }
    byName.get(name)?.push(e.value);
  }
  return order.map((name) => {
    const values = byName.get(name) ?? [];
    return { subjectName: name, average: mean(values), termCount: values.length };
  });
}

/**
 * Calificaciones: the child's real numeric grades from
 * `GET /api/v1/family/students/{id}/grades`, grouped by subject with a
 * real-average ring.
 *
 * Qualitative gap: the backend stores a single numeric `value` per entry and has
 * no qualitative (logrado/proceso) representation. Kínder is evaluated
 * qualitatively, so for a kínder child the screen shows a "Próximamente" state
 * rather than fabricating either numeric or qualitative marks.
 * TODO(pendiente-backend: qualitative grades) — render the real qualitative
 * boleta once the grades service exposes it.
 */
export function GradesScreen(): ReactElement {
  const { child, selectedChildId, childLoading } = useAppState();
  const view = toChildView(child);
  const { data, loading, error, reload } = useChildGrades(selectedChildId);
  const theme = useTheme();

  const entries = data ?? [];
  const subjects = useMemo(() => groupBySubject(entries), [entries]);
  const average = useMemo(() => mean(entries.map((e) => e.value)), [entries]);

  // Qualitative levels (kínder) have no backend representation — never fabricate.
  if (view.qualitative) {
    return (
      <>
        <ScreenTitle>Calificaciones</ScreenTitle>
        <Card padded style={{ marginTop: 12 }}>
          <EmptyState
            icon="sparkles"
            title="Próximamente"
            subtitle="La boleta cualitativa de este nivel estará disponible pronto."
          />
        </Card>
      </>
    );
  }

  if (error) {
    return (
      <>
        <ScreenTitle>Calificaciones</ScreenTitle>
        <AsyncErrorState
          title="No pudimos cargar las calificaciones"
          subtitle="Revisa tu conexión e inténtalo de nuevo."
          onRetry={reload}
        />
      </>
    );
  }

  if ((childLoading || loading) && data === null) {
    return (
      <>
        <ScreenTitle>Calificaciones</ScreenTitle>
        <AsyncLoading />
      </>
    );
  }

  if (entries.length === 0) {
    return (
      <>
        <ScreenTitle>Calificaciones</ScreenTitle>
        <Card padded style={{ marginTop: 12 }}>
          <EmptyState
            icon="book"
            title="Aún no hay calificaciones"
            subtitle="Aquí verás las materias y su promedio cuando la escuela las publique."
          />
        </Card>
      </>
    );
  }

  return (
    <>
      <ScreenTitle>Calificaciones</ScreenTitle>

      <Card style={styles.avgHero}>
        <Ring pct={average / 10} color={theme.colors.ok} track={theme.colors.surfaceSunken}>
          <Text accessibilityLabel="Promedio" style={[styles.avgNum, { color: theme.colors.ok, fontFamily: theme.fonts.uiExtra }]}>
            {fmt(average)}
          </Text>
          <Text style={[styles.avgOf, { color: theme.colors.text3, fontFamily: theme.fonts.ui }]}>de 10</Text>
        </Ring>
        <View style={{ flex: 1 }}>
          <Text style={[styles.heroTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Promedio general</Text>
          <Text style={[styles.heroSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
            {subjects.length} materias
          </Text>
        </View>
      </Card>

      <SectionLabel>Materias</SectionLabel>
      <Card>
        {subjects.map((s, i) => {
          const badge = numericBadge(theme.colors, s.average);
          return (
            <ListRow key={s.subjectName} divider={i > 0} accessibilityLabel={s.subjectName}>
              <View style={{ flex: 1 }}>
                <Text style={[styles.subjTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{s.subjectName}</Text>
                <Text style={[styles.subjSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
                  {s.termCount} {s.termCount === 1 ? "periodo" : "periodos"} · prom. {fmt(s.average)}
                </Text>
              </View>
              <View style={[styles.gradeBadge, { backgroundColor: badge.bg }]}>
                <Text style={[styles.gradeText, { color: badge.fg, fontFamily: theme.fonts.uiExtra }]}>{fmt(s.average)}</Text>
              </View>
            </ListRow>
          );
        })}
      </Card>
    </>
  );
}

const styles = StyleSheet.create({
  avgHero: { padding: 20, marginTop: 12, flexDirection: "row", alignItems: "center", gap: 18 },
  avgNum: { fontSize: 30, fontWeight: "800", letterSpacing: -0.5 },
  avgOf: { fontSize: 12, fontWeight: "700" },
  heroTitle: { fontSize: 19, fontWeight: "800" },
  heroSub: { fontSize: 13, fontWeight: "600", marginTop: 2 },
  subjTitle: { fontSize: 15, fontWeight: "800" },
  subjSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
  gradeBadge: { minWidth: 46, height: 46, borderRadius: 23, paddingHorizontal: 8, alignItems: "center", justifyContent: "center" },
  gradeText: { fontSize: 20, fontWeight: "800" },
});

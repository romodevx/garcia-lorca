import { useMemo, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyAttendanceRecord, FamilyAttendanceStatus } from "../api";
import type { ChipKind } from "../data/mock";
import { useAppState } from "../state/AppState";
import { useChildAttendance } from "../state/familyData";
import { useTheme, type ThemeColors } from "../theme";
import { Card, Chip, ListRow, ScreenTitle, SectionLabel, StatusDot } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";
import { formatDate } from "./format";

const STATUS_LABEL: Record<FamilyAttendanceStatus, string> = {
  present: "Asistió",
  absent: "Falta",
  late: "Retardo",
  excused: "Justificada",
};
const STATUS_CHIP: Record<FamilyAttendanceStatus, ChipKind> = {
  present: "ok",
  absent: "bad",
  late: "warn",
  excused: "warn",
};

function dotColor(colors: ThemeColors, status: FamilyAttendanceStatus): string {
  switch (status) {
    case "present":
      return colors.ok;
    case "absent":
      return colors.bad;
    default:
      return colors.warn;
  }
}

/** Counts of the exception statuses a parent watches, derived from the records. */
function tally(records: FamilyAttendanceRecord[]): { faltas: number; retardos: number; justificadas: number } {
  return {
    faltas: records.filter((r) => r.status === "absent").length,
    retardos: records.filter((r) => r.status === "late").length,
    justificadas: records.filter((r) => r.status === "excused").length,
  };
}

/**
 * Asistencia: the child's real attendance records from
 * `GET /api/v1/family/students/{id}/attendance` — the stored exceptions (absence,
 * retardo, justificante) with the counts derived from them. No attendance
 * percentage is shown: it can't be computed without the school-day calendar, and
 * a fabricated figure would mislead. Handles loading/error/empty.
 */
export function AttendanceScreen(): ReactElement {
  const { selectedChildId, childLoading } = useAppState();
  const { data, loading, error, reload } = useChildAttendance(selectedChildId);
  const theme = useTheme();

  const records = data ?? [];
  const stats = useMemo(() => tally(records), [records]);

  if (error) {
    return (
      <>
        <ScreenTitle>Asistencia</ScreenTitle>
        <AsyncErrorState
          title="No pudimos cargar la asistencia"
          subtitle="Revisa tu conexión e inténtalo de nuevo."
          onRetry={reload}
        />
      </>
    );
  }

  if ((childLoading || loading) && data === null) {
    return (
      <>
        <ScreenTitle>Asistencia</ScreenTitle>
        <AsyncLoading />
      </>
    );
  }

  return (
    <>
      <ScreenTitle>Asistencia</ScreenTitle>

      {records.length === 0 ? (
        <Card padded style={{ marginTop: 12 }}>
          <Text style={[styles.empty, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>
            Sin registros de asistencia
          </Text>
          <Text style={[styles.emptySub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
            Aquí verás faltas, retardos y justificantes.
          </Text>
        </Card>
      ) : (
        <>
          <Card style={{ marginTop: 12 }}>
            <View style={styles.statGrid}>
              <Stat value={stats.faltas} label="Faltas" color={theme.colors.bad} />
              <Stat value={stats.retardos} label="Retardos" color={theme.colors.warn} bordered />
              <Stat value={stats.justificadas} label="Justificadas" color={theme.colors.ok} />
            </View>
          </Card>

          <SectionLabel>Registros</SectionLabel>
          <Card>
            {records.map((r, i) => (
              <ListRow key={r.id} divider={i > 0}>
                <StatusDot color={dotColor(theme.colors, r.status)} size={12} />
                <View style={{ flex: 1 }}>
                  <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
                    {formatDate(r.date, "long")}
                  </Text>
                  {r.note ? (
                    <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{r.note}</Text>
                  ) : null}
                </View>
                <Chip kind={STATUS_CHIP[r.status]}>{STATUS_LABEL[r.status]}</Chip>
              </ListRow>
            ))}
          </Card>
        </>
      )}
    </>
  );
}

function Stat({ value, label, color, bordered }: { value: number; label: string; color: string; bordered?: boolean }): ReactElement {
  const theme = useTheme();
  return (
    <View
      style={[
        styles.stat,
        bordered ? { borderLeftWidth: 1, borderRightWidth: 1, borderColor: theme.colors.line } : null,
      ]}
    >
      <Text accessibilityLabel={label} style={[styles.statNum, { color, fontFamily: theme.fonts.display }]}>
        {String(value)}
      </Text>
      <Text style={[styles.statLab, { color: theme.colors.text3, fontFamily: theme.fonts.ui }]}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  statGrid: { flexDirection: "row" },
  stat: { flex: 1, alignItems: "center", paddingVertical: 14 },
  statNum: { fontSize: 26, fontWeight: "800", letterSpacing: -0.5 },
  statLab: { fontSize: 11.5, fontWeight: "700", marginTop: 6 },
  rowTitle: { fontSize: 15, fontWeight: "800", textTransform: "capitalize" },
  rowSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
  empty: { fontSize: 16, fontWeight: "800", textAlign: "center" },
  emptySub: { fontSize: 13, fontWeight: "600", marginTop: 4, textAlign: "center" },
});

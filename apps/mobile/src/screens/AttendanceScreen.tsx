import { useMemo, useState, type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { attendance, type AttendanceStatus } from "../data/mock";
import { Icon } from "../icons";
import { ExcuseSheet } from "../sheets/ExcuseSheet";
import { useAppState } from "../state/AppState";
import { useToast } from "../state/Toast";
import { useTheme, type ThemeColors } from "../theme";
import { Button, Card, Chip, ListRow, ScreenTitle, Segment, StatusDot } from "../ui";
import type { ChipKind } from "../data/mock";

const STATUS_LABEL: Record<string, string> = {
  present: "Asistió",
  absent: "Falta",
  excused: "Justificada",
  retardo: "Retardo",
  holiday: "Sin clases",
};
const STATUS_CHIP: Record<string, ChipKind> = {
  present: "ok",
  absent: "bad",
  excused: "warn",
  retardo: "warn",
  holiday: "neutral",
};

const DOW = ["L", "M", "M", "J", "V", "S", "D"];

function cellColors(colors: ThemeColors, status: AttendanceStatus | undefined): { bg: string; fg: string } {
  switch (status) {
    case "present":
      return { bg: colors.okBg, fg: colors.ok };
    case "absent":
      return { bg: colors.badBg, fg: colors.bad };
    case "excused":
    case "retardo":
      return { bg: colors.warnBg, fg: colors.warn };
    default:
      return { bg: "transparent", fg: colors.text3 };
  }
}

function dotColor(colors: ThemeColors, status: AttendanceStatus): string {
  switch (status) {
    case "present":
      return colors.ok;
    case "absent":
      return colors.bad;
    case "excused":
    case "retardo":
      return colors.warn;
    default:
      return colors.text3;
  }
}

/**
 * Asistencia: monthly stats, an unjustified-absence callout, a calendar/list
 * segmented view, and one-tap "Justificar" that opens the excuse sheet and —
 * on confirm — flips the day to excused (live, via app state) and toasts.
 */
export function AttendanceScreen(): ReactElement {
  const theme = useTheme();
  const { attendanceStatus, excuseDay } = useAppState();
  const { fireToast } = useToast();
  const [view, setView] = useState<"cal" | "list">("cal");
  const [excusing, setExcusing] = useState<{ day: number; label: string } | null>(null);

  // First still-unjustified absence (label comes from the log entry).
  const pending = useMemo(() => {
    for (const entry of attendance.log) {
      if (attendanceStatus(entry.day) === "absent") {
        return { day: entry.day, label: entry.date };
      }
    }
    return null;
  }, [attendanceStatus]);

  const cells = useMemo(() => {
    const lead = attendance.firstDow === 0 ? 6 : attendance.firstDow - 1;
    const out: (number | null)[] = [];
    for (let i = 0; i < lead; i += 1) out.push(null);
    for (let d = 1; d <= 30; d += 1) out.push(d);
    return out;
  }, []);

  const confirmExcuse = (): void => {
    if (excusing) {
      excuseDay(excusing.day);
      setExcusing(null);
      fireToast("Justificante enviado a la escuela");
    }
  };

  return (
    <>
      <ScreenTitle>Asistencia</ScreenTitle>

      <Card style={{ marginTop: 12 }}>
        <View style={styles.statGrid}>
          <Stat value={`${attendance.stats.pct}%`} label="Asistencia" color={theme.colors.ok} />
          <Stat value={`${attendance.stats.faltas}`} label="Faltas" color={theme.colors.bad} bordered />
          <Stat value={`${attendance.stats.retardos}`} label="Retardos" color={theme.colors.warn} />
        </View>
      </Card>

      {pending ? (
        <Card padded style={styles.callout}>
          <View style={[styles.alertCircle, { backgroundColor: theme.colors.redSoft }]}>
            <Icon name="alert" size={22} color={theme.colors.tintRedInk} />
          </View>
          <View style={{ flex: 1, minWidth: 0 }}>
            <Text style={[styles.calloutTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
              1 falta sin justificar
            </Text>
            <Text style={[styles.calloutSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
              {pending.label}
            </Text>
          </View>
          <Button
            label="Justificar"
            small
            onPress={() => setExcusing(pending)}
            style={styles.justifyBtn}
          />
        </Card>
      ) : null}

      <View style={{ marginTop: 14 }}>
        <Segment
          options={[
            { value: "cal", label: "Calendario" },
            { value: "list", label: "Lista" },
          ]}
          value={view}
          onChange={setView}
        />
      </View>

      {view === "cal" ? (
        <Card padded style={{ marginTop: 14 }}>
          <View style={styles.calHead}>
            <Text style={[styles.calMonth, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>
              {attendance.monthLabel}
            </Text>
          </View>
          <View style={styles.calGrid}>
            {DOW.map((d, i) => (
              <View key={i} style={styles.calCellWrap}>
                <Text style={[styles.dow, { color: theme.colors.text3 }]}>{d}</Text>
              </View>
            ))}
          </View>
          <View style={styles.calGrid}>
            {cells.map((d, i) => {
              if (d === null) return <View key={`e${i}`} style={styles.calCellWrap} />;
              const status = attendanceStatus(d);
              const c = cellColors(theme.colors, status);
              const isToday = d === attendance.todayDate;
              const tappable = status === "absent";
              return (
                <View key={d} style={styles.calCellWrap}>
                  <Pressable
                    accessibilityRole={tappable ? "button" : undefined}
                    accessibilityLabel={tappable ? `Justificar día ${d}` : `Día ${d}`}
                    disabled={!tappable}
                    onPress={() => tappable && setExcusing({ day: d, label: pending?.label ?? `día ${d}` })}
                    style={[
                      styles.calCell,
                      { backgroundColor: c.bg },
                      isToday ? { borderColor: theme.colors.primary, borderWidth: 2 } : null,
                    ]}
                  >
                    <Text style={[styles.calNum, { color: c.fg, fontFamily: theme.fonts.uiExtra }]}>{d}</Text>
                  </Pressable>
                </View>
              );
            })}
          </View>
          <View style={styles.legend}>
            {(
              [
                ["present", "Asistió"],
                ["absent", "Falta"],
                ["excused", "Justificada / retardo"],
              ] as const
            ).map(([s, l]) => (
              <View key={s} style={styles.legendItem}>
                <View style={[styles.legendSwatch, { backgroundColor: dotColor(theme.colors, s) }]} />
                <Text style={[styles.legendText, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>{l}</Text>
              </View>
            ))}
          </View>
          <Text style={[styles.hint, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>
            Toca un día con falta para justificarlo
          </Text>
        </Card>
      ) : (
        <Card style={{ marginTop: 14 }}>
          {attendance.log.map((e, i) => {
            const status = attendanceStatus(e.day) ?? e.status;
            return (
              <ListRow key={e.date} divider={i > 0}>
                <StatusDot color={dotColor(theme.colors, status)} size={12} />
                <View style={{ flex: 1 }}>
                  <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{e.date}</Text>
                  <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{e.note}</Text>
                </View>
                {status === "absent" ? (
                  <Button label="Justificar" small onPress={() => setExcusing({ day: e.day, label: e.date })} style={styles.justifyBtn} />
                ) : (
                  <Chip kind={STATUS_CHIP[status] ?? "neutral"}>{STATUS_LABEL[status] ?? status}</Chip>
                )}
              </ListRow>
            );
          })}
        </Card>
      )}

      <ExcuseSheet
        visible={excusing !== null}
        dayLabel={excusing?.label ?? ""}
        onClose={() => setExcusing(null)}
        onConfirm={confirmExcuse}
      />
    </>
  );
}

function Stat({ value, label, color, bordered }: { value: string; label: string; color: string; bordered?: boolean }): ReactElement {
  const theme = useTheme();
  return (
    <View
      style={[
        styles.stat,
        bordered ? { borderLeftWidth: 1, borderRightWidth: 1, borderColor: theme.colors.line } : null,
      ]}
    >
      <Text style={[styles.statNum, { color, fontFamily: theme.fonts.display }]}>{value}</Text>
      <Text style={[styles.statLab, { color: theme.colors.text3, fontFamily: theme.fonts.ui }]}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  statGrid: { flexDirection: "row" },
  stat: { flex: 1, alignItems: "center", paddingVertical: 14 },
  statNum: { fontSize: 26, fontWeight: "800", letterSpacing: -0.5 },
  statLab: { fontSize: 11.5, fontWeight: "700", marginTop: 6 },
  callout: { marginTop: 12, flexDirection: "row", alignItems: "center", gap: 12 },
  alertCircle: { width: 44, height: 44, borderRadius: 22, alignItems: "center", justifyContent: "center" },
  calloutTitle: { fontSize: 14.5, fontWeight: "800" },
  calloutSub: { fontSize: 13, fontWeight: "600", marginTop: 1 },
  justifyBtn: { width: "auto", paddingHorizontal: 16 },
  calHead: { alignItems: "center", paddingBottom: 14 },
  calMonth: { fontSize: 18, fontWeight: "700", textTransform: "capitalize" },
  calGrid: { flexDirection: "row", flexWrap: "wrap" },
  calCellWrap: { width: `${100 / 7}%`, padding: 2, alignItems: "center" },
  dow: { fontSize: 11, fontWeight: "800", textTransform: "uppercase", paddingBottom: 4 },
  calCell: { width: "100%", aspectRatio: 1, borderRadius: 999, alignItems: "center", justifyContent: "center" },
  calNum: { fontSize: 14, fontWeight: "800" },
  legend: { flexDirection: "row", flexWrap: "wrap", gap: 14, paddingTop: 14 },
  legendItem: { flexDirection: "row", alignItems: "center", gap: 6 },
  legendSwatch: { width: 12, height: 12, borderRadius: 5 },
  legendText: { fontSize: 12, fontWeight: "700" },
  hint: { fontSize: 12, fontWeight: "700", marginTop: 12, textAlign: "center" },
  rowTitle: { fontSize: 15, fontWeight: "800" },
  rowSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
});

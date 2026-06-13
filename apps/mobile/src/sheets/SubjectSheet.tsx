import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { NumericSubject } from "../data/mock";
import { useTheme, type ThemeColors } from "../theme";
import { BottomSheet, Button, ProgressBar, TintIcon } from "../ui";

export interface SubjectSheetProps {
  visible: boolean;
  subject: NumericSubject | null;
  onClose: () => void;
}

function gradeColors(colors: ThemeColors, g: number): { bg: string; fg: string } {
  if (g >= 9) return { bg: colors.okBg, fg: colors.ok };
  if (g >= 8) return { bg: colors.primarySoft, fg: colors.primaryInk };
  if (g >= 7) return { bg: colors.warnBg, fg: colors.warn };
  return { bg: colors.badBg, fg: colors.bad };
}

function fmt(n: number): string {
  return n === 10 ? "10" : n.toFixed(1);
}

/** Subject detail sheet: per-parcial breakdown with bars + period average. */
export function SubjectSheet({ visible, subject, onClose }: SubjectSheetProps): ReactElement | null {
  const theme = useTheme();
  if (subject === null) {
    return <BottomSheet visible={visible} onClose={onClose} />;
  }
  const badge = gradeColors(theme.colors, subject.grade);

  return (
    <BottomSheet visible={visible} onClose={onClose}>
      <View style={styles.head}>
        <TintIcon name={subject.icon} tint={subject.tint} size={50} icon={26} />
        <View style={{ flex: 1 }}>
          <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{subject.name}</Text>
          <Text style={[styles.sub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>Promedio del periodo</Text>
        </View>
        <View style={[styles.gradeBadge, { backgroundColor: badge.bg }]}>
          <Text style={[styles.gradeText, { color: badge.fg, fontFamily: theme.fonts.uiExtra }]}>{fmt(subject.grade)}</Text>
        </View>
      </View>

      <Text style={[styles.section, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>POR PARCIAL</Text>

      <View style={[styles.card, { backgroundColor: theme.colors.surface, borderColor: theme.colors.line }]}>
        {subject.terms.map((t, i) => {
          const fill = t >= 9 ? theme.colors.ok : t >= 8 ? theme.palette.lime : theme.colors.warn;
          return (
            <View key={i} style={[styles.termRow, i > 0 ? { borderTopColor: theme.colors.line, borderTopWidth: StyleSheet.hairlineWidth } : null]}>
              <Text style={[styles.termLabel, { color: theme.colors.text, fontFamily: theme.fonts.ui }]}>{i + 1}.er parcial</Text>
              <ProgressBar pct={t / 10} color={fill} style={styles.bar} />
              <Text style={[styles.termGrade, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>{fmt(t)}</Text>
            </View>
          );
        })}
      </View>

      <Button label="Cerrar" variant="ghost" onPress={onClose} style={{ marginTop: 18 }} />
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  head: { flexDirection: "row", alignItems: "center", gap: 13, marginBottom: 6 },
  title: { fontSize: 19, fontWeight: "800", letterSpacing: -0.3 },
  sub: { fontSize: 13, fontWeight: "600", marginTop: 2 },
  gradeBadge: { minWidth: 56, height: 56, borderRadius: 28, alignItems: "center", justifyContent: "center", paddingHorizontal: 8 },
  gradeText: { fontSize: 24, fontWeight: "800" },
  section: { fontSize: 12.5, fontWeight: "800", letterSpacing: 0.4, marginTop: 14, marginBottom: 6 },
  card: { borderWidth: 1, borderRadius: 20, overflow: "hidden" },
  termRow: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 14, paddingHorizontal: 16 },
  termLabel: { fontSize: 15, fontWeight: "700", flex: 1 },
  bar: { flex: 1, maxWidth: 120 },
  termGrade: { fontSize: 16, fontWeight: "700", minWidth: 36, textAlign: "right" },
});

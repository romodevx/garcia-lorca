import { router } from "expo-router";
import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { money, today } from "../data/mock";
import { Icon } from "../icons";
import { useAppState } from "../state/AppState";
import { useTheme } from "../theme";
import { Card, Chip, SectionLabel, TintIcon } from "../ui";

/**
 * Inicio: greeting + actionable summary cards. Each card routes straight to its
 * detail (asistencia, cuenta, avisos, eventos). The balance card flips to a
 * "settled" state and hides "Pagar ahora" once the payment has been made.
 */
export function HomeScreen(): ReactElement {
  const theme = useTheme();
  const { child, paid, unreadCount } = useAppState();
  // today.attendance / today.balance / today.nextEvent are still static mock:
  // TODO(pendiente-backend: GET /api/v1/family/students/{id}/attendance | .../account | agenda)
  // — static placeholder data. Do not delete. See .claude/docs/PLAN-mock-data-audit.md #B3/#B4/#B6.
  const att = today.attendance;
  const bal = today.balance;

  return (
    <>
      <View style={styles.greetRow}>
        <View>
          <Text style={[styles.hello, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
            Hola, {child.parent}
          </Text>
          <Text style={[styles.date, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>
            {today.dateLabel} · Ciclo 2025–2026
          </Text>
        </View>
      </View>

      <SectionLabel>Hoy</SectionLabel>

      {/* Today's attendance → Asistencia */}
      <Card onPress={() => router.push("/(tabs)/asistencia")} accessibilityLabel="Asistencia de hoy" style={styles.hero}>
        <View style={styles.heroTop}>
          <View style={[styles.statusCircle, { backgroundColor: theme.colors.primarySoft }]}>
            <Icon name="check" size={28} stroke={3} color={theme.colors.tintLimeInk} />
          </View>
          <View style={styles.heroMain}>
            <Text style={[styles.heroLabel, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>Asistencia de hoy</Text>
            <Text style={[styles.heroValue, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{att.label}</Text>
            <Text style={[styles.heroSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{att.detail}</Text>
          </View>
          <Icon name="chevron-right" size={20} stroke={2.4} color={theme.colors.text3} />
        </View>
      </Card>

      {/* Balance → Cuenta (with Pagar ahora) */}
      <Card onPress={() => router.push("/(tabs)/cuenta")} accessibilityLabel="Saldo" style={[styles.hero, { marginTop: 12 }]}>
        <View style={styles.heroTop}>
          <View style={[styles.statusCircle, { backgroundColor: paid ? theme.colors.primarySoft : theme.colors.orangeSoft }]}>
            <Icon name="wallet" size={26} color={paid ? theme.colors.tintLimeInk : theme.colors.tintOrangeInk} />
          </View>
          <View style={styles.heroMain}>
            <Text style={[styles.heroLabel, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>
              {paid ? "Estado de cuenta" : "Saldo por pagar"}
            </Text>
            <Text style={[styles.heroValue, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
              {paid ? money(0) : money(bal.amount)}
            </Text>
            <View style={{ marginTop: 7 }}>
              {paid ? (
                <Chip kind="ok" icon="check">Al corriente</Chip>
              ) : (
                <Chip kind="warn" icon="clock">{`${bal.label} · ${bal.due}`}</Chip>
              )}
            </View>
          </View>
          <Icon name="chevron-right" size={20} stroke={2.4} color={theme.colors.text3} />
        </View>
      </Card>

      {/* Avisos (real notifications). The former "próximo evento" tile + the
          "Próximos eventos" list are hidden: the comms agenda has no backend yet
          (Phase 9d), so `today.nextEvent` / `events` are static mock and showing
          them would fake data. They stay in mock.ts, just not rendered. */}
      <Card
        onPress={() => router.push("/(tabs)/avisos")}
        accessibilityLabel="Avisos"
        padded
        style={{ marginTop: 12 }}
      >
        <View style={styles.tileTop}>
          <TintIcon name="megaphone" tint="purple" />
          <Chip kind="bad">{`${unreadCount} nuevos`}</Chip>
        </View>
        <Text style={[styles.tileTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Avisos</Text>
        <Text style={[styles.heroSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>Sin leer</Text>
      </Card>
    </>
  );
}

const styles = StyleSheet.create({
  greetRow: { paddingHorizontal: 4, paddingTop: 8, paddingBottom: 2 },
  hello: { fontSize: 22, fontWeight: "800", letterSpacing: -0.3 },
  date: { fontSize: 13, fontWeight: "700", marginTop: 1 },
  hero: { padding: 18 },
  heroTop: { flexDirection: "row", alignItems: "center", gap: 12 },
  statusCircle: { width: 52, height: 52, borderRadius: 18, alignItems: "center", justifyContent: "center" },
  heroMain: { flex: 1, minWidth: 0 },
  heroLabel: { fontSize: 13, fontWeight: "700" },
  heroValue: { fontSize: 25, fontWeight: "800", letterSpacing: -0.5, marginTop: 1 },
  heroSub: { fontSize: 13, fontWeight: "600", marginTop: 2 },
  tileTop: { flexDirection: "row", justifyContent: "space-between", alignItems: "flex-start" },
  tileTitle: { fontSize: 17, fontWeight: "800", marginTop: 12 },
});

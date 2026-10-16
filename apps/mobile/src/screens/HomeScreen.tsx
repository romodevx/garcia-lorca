import { router } from "expo-router";
import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { Icon } from "../icons";
import { useAppState } from "../state/AppState";
import { toChildView } from "../state/childView";
import { useRefreshHandler } from "../components/refresh";
import { useChildStatement } from "../state/familyData";
import { useTheme } from "../theme";
import { Card, Chip, SectionLabel, TintIcon } from "../ui";
import { AsyncErrorState } from "./AsyncStates";
import { todayHeadline } from "./attendanceView";
import { useSchoolMonth } from "./attendance/useSchoolMonth";
import { pesos } from "./format";

/**
 * Inicio: greeting + actionable summary cards, each routing to its detail. The
 * attendance and balance summaries are derived from the SAME real family reads
 * the detail screens use ( `/family/.../attendance` and `/.../statement` ); when
 * there is no data yet the cards show a neutral state rather than a fabricated one.
 * The agenda/next-event tile is intentionally absent — it has no backend.
 */
export function HomeScreen(): ReactElement {
  const theme = useTheme();
  const { child, selectedChildId, unreadCount, childError, refreshChildren, refreshNotifications } = useAppState();
  const view = toChildView(child);
  const account = useChildStatement(selectedChildId);
  const { resource: attendance, today: todayRecord } = useSchoolMonth(selectedChildId);
  useRefreshHandler(async () => {
    await Promise.all([account.reload(), attendance.reload(), refreshNotifications()]);
  });

  // The child bootstrap failed: there is no child to summarize. Surface the error
  // (retrying the child load) instead of the neutral "Sin novedades / —" cards,
  // which would read as a fabricated healthy state (bug_005).
  if (childError) {
    return (
      <AsyncErrorState
        title="No pudimos cargar la información"
        subtitle="Revisa tu conexión e inténtalo de nuevo."
        onRetry={refreshChildren}
      />
    );
  }

  const balanceCents = account.data?.totals.balanceCents ?? null;
  const settled = balanceCents !== null && balanceCents <= 0;
  // Below zero the school owes the family (T5): credit, as a positive amount.
  const inCredit = balanceCents !== null && balanceCents < 0;
  const balanceLabel = inCredit ? "Saldo a favor" : settled ? "Estado de cuenta" : "Saldo por pagar";

  const dateLabel = new Date().toLocaleDateString("es-MX", { weekday: "long", day: "numeric", month: "long" });

  return (
    <>
      <View style={styles.greetRow}>
        <Text style={[styles.hello, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
          {view.present ? `Hola, familia ${view.short}` : "Hola"}
        </Text>
        <Text style={[styles.date, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>
          {dateLabel}
        </Text>
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
            <Text style={[styles.heroValue, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
              {todayHeadline(todayRecord).text}
            </Text>
            {todayRecord?.note ? (
              <Text style={[styles.heroSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{todayRecord.note}</Text>
            ) : null}
          </View>
          <Icon name="chevron-right" size={20} stroke={2.4} color={theme.colors.text3} />
        </View>
      </Card>

      {/* Balance → Cuenta */}
      <Card onPress={() => router.push("/(tabs)/cuenta")} accessibilityLabel="Saldo" style={[styles.hero, { marginTop: 12 }]}>
        <View style={styles.heroTop}>
          <View style={[styles.statusCircle, { backgroundColor: settled ? theme.colors.primarySoft : theme.colors.orangeSoft }]}>
            <Icon name="wallet" size={26} color={settled ? theme.colors.tintLimeInk : theme.colors.tintOrangeInk} />
          </View>
          <View style={styles.heroMain}>
            <Text style={[styles.heroLabel, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>
              {balanceLabel}
            </Text>
            <Text style={[styles.heroValue, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
              {balanceCents === null ? "—" : pesos(Math.abs(balanceCents))}
            </Text>
            <View style={{ marginTop: 7 }}>
              {balanceCents === null ? null : settled ? (
                <Chip kind="ok" icon="check">Al corriente</Chip>
              ) : (
                <Chip kind="warn" icon="clock">Pendiente</Chip>
              )}
            </View>
          </View>
          <Icon name="chevron-right" size={20} stroke={2.4} color={theme.colors.text3} />
        </View>
      </Card>

      {/* Avisos (real notifications) */}
      <Card onPress={() => router.push("/(tabs)/avisos")} accessibilityLabel="Avisos" padded style={{ marginTop: 12 }}>
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
  date: { fontSize: 13, fontWeight: "700", marginTop: 1, textTransform: "capitalize" },
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

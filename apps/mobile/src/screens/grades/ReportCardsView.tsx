import { router } from "expo-router";
import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { ReportCardSummary } from "../../api/reportCards";
import { useRefreshHandler } from "../../components/refresh";
import { Icon } from "../../icons";
import { useAppState } from "../../state/AppState";
import { useChildReportCards } from "../../state/familyData";
import { useTheme } from "../../theme";
import { Card, EmptyState, ListRow, TintIcon } from "../../ui";
import { AsyncErrorState, AsyncLoading, StaleNotice } from "../AsyncStates";
import { formatDate } from "../format";
import { averageText, reportCardLabel, reportCardTitle, versionBadge } from "./reportCardView";

const HTTP_NOT_FOUND = 404;

/** Opens one boleta's preview (also the target of a boleta push). */
export function openReportCard(id: string): void {
  router.push({ pathname: "/boleta/[id]", params: { id } });
}

/**
 * "Boletas": the child's issued trimester report cards, newest term first —
 * one row each with its version (a corrected one says "Actualizada"), general
 * average and issue date. Tapping one opens its preview and PDF.
 */
export function ReportCardsView(): ReactElement | null {
  const { selectedChildId, childLoading } = useAppState();
  const { data, loading, error, errorStatus, reload } = useChildReportCards(selectedChildId);
  useRefreshHandler(reload);

  if (error && data === null) {
    const notFound = errorStatus === HTTP_NOT_FOUND;
    return (
      <AsyncErrorState
        title={notFound ? "No encontramos a este alumno" : "No pudimos cargar las boletas"}
        subtitle={notFound ? "Si crees que es un error, avisa a la escuela." : "Revisa tu conexión e inténtalo de nuevo."}
        onRetry={reload}
      />
    );
  }
  if (data === null) {
    return childLoading || loading ? <AsyncLoading /> : null;
  }
  if (data.length === 0) {
    return (
      <Card padded style={styles.top}>
        <EmptyState
          icon="document"
          title="Aún no hay boletas emitidas"
          subtitle="Cuando la escuela cierre un trimestre, su boleta aparecerá aquí."
        />
      </Card>
    );
  }
  return (
    <>
      {error ? <StaleNotice /> : null}
      <Card style={styles.top}>
        {data.map((card, i) => (
          <ListRow key={card.id} divider={i > 0} onPress={() => openReportCard(card.id)} accessibilityLabel={reportCardLabel(card)} accessibilityHint="Ver la boleta">
            <ReportCardRow card={card} />
          </ListRow>
        ))}
      </Card>
    </>
  );
}

function ReportCardRow({ card }: { card: ReportCardSummary }): ReactElement {
  const theme = useTheme();
  const badge = versionBadge(card.version);
  const average = averageText(card);
  const issued = card.issuedAt ? formatDate(card.issuedAt, "long") : "";
  const sub = { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi };
  return (
    <>
      <TintIcon name="document" tint="forest" />
      <View style={styles.text}>
        <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{reportCardTitle(card)}</Text>
        {badge === null ? null : (
          <View style={[styles.badge, { backgroundColor: theme.colors.warnBg }]}>
            <Text style={[styles.badgeText, { color: theme.colors.warn, fontFamily: theme.fonts.uiExtra }]}>{badge}</Text>
          </View>
        )}
        <Text style={[styles.sub, sub]}>
          {average === null ? "Sin calificaciones" : "Promedio general "}
          {average === null ? null : <Text style={{ color: theme.colors.text, fontFamily: theme.fonts.uiExtra }}>{average}</Text>}
        </Text>
        {issued ? <Text style={[styles.sub, sub]}>{`Emitida el ${issued}`}</Text> : null}
      </View>
      <Icon name="chevron-right" size={20} color={theme.colors.text3} />
    </>
  );
}

const styles = StyleSheet.create({
  top: { marginTop: 14 },
  text: { flex: 1, gap: 3 },
  title: { fontSize: 17, fontWeight: "800" },
  badge: { alignSelf: "flex-start", borderRadius: 999, paddingHorizontal: 10, paddingVertical: 3, marginTop: 2 },
  badgeText: { fontSize: 14, fontWeight: "800" },
  sub: { fontSize: 15, fontWeight: "600" },
});

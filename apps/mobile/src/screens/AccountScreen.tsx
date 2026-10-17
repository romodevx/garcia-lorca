import { useFlag } from "@garcia-lorca/ui/flags";
import { router } from "expo-router";
import { useMemo, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { statementPdfPath } from "../api";
import { useAppState } from "../state/AppState";
import { useChildStatement } from "../state/familyData";
import { useRefreshHandler } from "../components/refresh";
import { useTheme } from "../theme";
import { Button, Card, Chip, EmptyState, ListRow, ScreenTitle, SectionLabel, TintIcon } from "../ui";
import { MovementList } from "./account/MovementList";
import { splitLedger } from "./account/ledgerView";
import { useReceiptDownload } from "./account/useReceiptDownload";
import { AsyncErrorState, AsyncLoading, StaleNotice } from "./AsyncStates";
import { usePdfDownload, type PdfDownloadState } from "./grades/usePdfDownload";
import { formatDayWithYear, pesos, totalBalanceLine } from "./format";

const PDF_MESSAGES = {
  network: "No pudimos descargar el estado de cuenta. Revisa tu conexión e inténtalo de nuevo.",
  server: "La escuela no pudo entregar el estado de cuenta en este momento. Inténtalo más tarde.",
  storage: "No pudimos guardar el estado de cuenta en el teléfono. Libera espacio e inténtalo de nuevo.",
  not_found: "El estado de cuenta no está disponible.",
  session: "Tu sesión terminó. Vuelve a iniciar sesión para descargar el estado de cuenta.",
};

/**
 * Cuenta/Saldo: the child's account statement from
 * `GET /api/v1/family/students/{id}/statement` — the SAME ledger the school
 * reads (finance.GetStatement, T7): every movement with its running balance,
 * the pending balance and today's figure. The screen renders it newest first
 * and derives nothing. Read-only; never fabricates a settled state.
 */
export function AccountScreen(): ReactElement {
  const { selectedChildId, childLoading, childError, refreshChildren } = useAppState();
  const { data, loading, error, reload } = useChildStatement(selectedChildId);
  useRefreshHandler(reload);
  const cardEnabled = useFlag("module_student_card");
  const theme = useTheme();
  const pdfTarget = useMemo(
    () =>
      selectedChildId === null
        ? null
        : {
            path: statementPdfPath(selectedChildId),
            studentId: selectedChildId,
            fallbackName: "estado-de-cuenta.pdf",
            title: "Estado de cuenta",
            messages: PDF_MESSAGES,
          },
    [selectedChildId],
  );
  const pdf = usePdfDownload(pdfTarget);
  const receipt = useReceiptDownload(selectedChildId);

  // The child bootstrap failed, so there is no child to read against. Surface that
  // error (retrying the child load) instead of falling through to a fabricated
  // settled "$0.00 / Al corriente" state (bug_005).
  if (childError) {
    return (
      <>
        <ScreenTitle>Cuenta</ScreenTitle>
        <AsyncErrorState
          title="No pudimos cargar tu cuenta"
          subtitle="Revisa tu conexión e inténtalo de nuevo."
          onRetry={refreshChildren}
        />
      </>
    );
  }

  // A failed refresh over data already on screen keeps the data (inline notice below).
  if (error && data === null) {
    return (
      <>
        <ScreenTitle>Cuenta</ScreenTitle>
        <AsyncErrorState
          title="No pudimos cargar tu cuenta"
          subtitle="Revisa tu conexión e inténtalo de nuevo."
          onRetry={reload}
        />
      </>
    );
  }

  if ((childLoading || loading) && data === null) {
    return (
      <>
        <ScreenTitle>Cuenta</ScreenTitle>
        <AsyncLoading />
      </>
    );
  }

  // What the school is asking for TODAY leads; the pending balance sits beside
  // it. Both are true at once, and a parent shown only the balance is told a
  // number nobody is asking them for — they are also the one person who cannot
  // walk to the office and ask which is which.
  //
  // An EMPTY asOf means the day is unknown. Rendering the day's figure as $0.00
  // there would tell a family that owes money that they are settled — the same
  // fabricated "Al corriente" this screen refuses when the child bootstrap
  // fails (bug_005). So the hero falls back to the balance.
  const hasToday = (data?.asOf ?? "") !== "";
  const balanceCents = data?.totals.balanceCents ?? 0;
  const heroCents = hasToday ? (data?.dueTodayCents ?? 0) : balanceCents;
  const settled = heroCents <= 0;
  // Without the day the hero IS the balance, and a balance below zero is the
  // school owing the family (T5): credit, in words and as a positive amount.
  const heroIsCredit = !hasToday && balanceCents < 0;
  // dueTodayCents is what makes the month current, early-payment reward
  // included (ISSUE-T14-106) — not what is overdue.
  const heroLabel = hasToday ? "Para estar al corriente" : heroIsCredit ? "Saldo a favor" : settled ? "Saldo actual" : "Saldo por pagar";
  // What has happened newest first, the cycle's later months below (T14-106).
  const ledger = splitLedger(data?.movements ?? [], data?.asOf ?? "");

  return (
    <>
      <ScreenTitle>Cuenta</ScreenTitle>
      {error ? <StaleNotice /> : null}

      <Card style={[styles.balanceHero, { marginTop: 12 }]}>
        <Text style={[styles.cap, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>
          {heroLabel}
        </Text>
        <Text
          accessibilityLabel="Saldo"
          style={[styles.amount, { color: settled ? theme.colors.ok : theme.colors.text, fontFamily: theme.fonts.display }]}
        >
          {pesos(heroIsCredit ? -heroCents : Math.max(heroCents, 0))}
        </Text>
        {hasToday && (
          <>
            <Text style={[styles.year, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>
              {totalBalanceLine(balanceCents)}
            </Text>
            <Text
              accessibilityLabel="Detalle del saldo"
              style={[styles.detail, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}
            >
              {`${(data?.surchargeCents ?? 0) > 0 ? `Incluye ${pesos(data?.surchargeCents ?? 0)} de recargo. ` : ""}` +
                `${(data?.earlyDiscountCents ?? 0) > 0 ? `Ya con ${pesos(data?.earlyDiscountCents ?? 0)} de pronto pago. ` : ""}` +
                `Al ${formatDayWithYear(data?.asOf ?? "")}.`}
            </Text>
          </>
        )}
        <View style={styles.center}>
          {settled ? (
            <Chip kind="ok" icon="check">Al corriente</Chip>
          ) : (
            <Chip kind="warn" icon="clock">Pendiente</Chip>
          )}
        </View>
      </Card>

      {cardEnabled && (
        // The child's virtual card (task 13), behind module_student_card.
        <Card onPress={() => router.push("/credencial")} accessibilityLabel="Credencial" style={{ marginTop: 12 }}>
          <ListRow>
            <TintIcon name="user" tint="forest" />
            <Text style={[styles.cardLink, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Credencial</Text>
          </ListRow>
        </Card>
      )}

      <SectionLabel>Movimientos</SectionLabel>
      {ledger.happened.length === 0 && ledger.upcoming.length === 0 ? (
        <EmptyState icon="receipt" title="Sin movimientos" subtitle="Aquí verás los cargos y pagos de tu cuenta." />
      ) : null}
      <PdfStatus state={receipt.state} />
      {ledger.happened.length > 0 ? <MovementList movements={ledger.happened} onOpenReceipt={receipt.openReceipt} /> : null}
      {ledger.upcoming.length > 0 ? (
        <>
          <SectionLabel>Próximos cargos</SectionLabel>
          <MovementList movements={ledger.upcoming} />
        </>
      ) : null}

      <View style={styles.action}>
        <Button label="Descargar estado de cuenta" icon="download" onPress={pdf.start} accessibilityLabel="Descargar estado de cuenta" />
        <PdfStatus state={pdf.state} />
      </View>
    </>
  );
}

/** A download's progress or failure, in words; nothing while idle. */
function PdfStatus({ state }: { state: PdfDownloadState }): ReactElement | null {
  const theme = useTheme();
  if (state.status === "downloading") {
    return (
      <Text accessibilityLiveRegion="polite" style={[styles.status, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
        Descargando…
      </Text>
    );
  }
  if (state.status === "failed") {
    return (
      <Text accessibilityRole="alert" style={[styles.status, { color: theme.colors.bad, fontFamily: theme.fonts.uiSemi }]}>
        {state.message}
      </Text>
    );
  }
  return null;
}

const styles = StyleSheet.create({
  balanceHero: { padding: 22, alignItems: "center" },
  cardLink: { fontSize: 16, marginLeft: 12 },
  year: { fontSize: 13, marginTop: 2 },
  detail: { fontSize: 12, marginTop: 6, textAlign: "center" },
  cap: { fontSize: 13, fontWeight: "700", marginBottom: 4 },
  amount: { fontSize: 44, fontWeight: "800", letterSpacing: -1.5 },
  center: { alignItems: "center", marginTop: 12 },
  action: { marginTop: 18, gap: 10 },
  status: { fontSize: 16, fontWeight: "600", textAlign: "center" },
});

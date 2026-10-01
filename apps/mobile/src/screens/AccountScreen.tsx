import { useFlag } from "@garcia-lorca/ui/flags";
import { router } from "expo-router";
import { useMemo, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { statementPdfPath, type FamilyMovement, type MovementKind } from "../api";
import type { IconName } from "../icons";
import { useAppState } from "../state/AppState";
import { useChildStatement } from "../state/familyData";
import { useRefreshHandler } from "../components/refresh";
import { useTheme, type TintName } from "../theme";
import { Button, Card, Chip, EmptyState, ListRow, ScreenTitle, SectionLabel, TintIcon } from "../ui";
import { AsyncErrorState, AsyncLoading, StaleNotice } from "./AsyncStates";
import { usePdfDownload } from "./grades/usePdfDownload";
import { formatDate, pesos } from "./format";

/** How each kind of ledger line looks. The ledger itself is finance's (T7). */
const KIND_LOOK: Record<MovementKind, { icon: IconName; tint: TintName; label: string; credit: boolean }> = {
  charge: { icon: "receipt", tint: "orange", label: "Cargo", credit: false },
  surcharge: { icon: "alert", tint: "orange", label: "Recargo", credit: false },
  discount: { icon: "check-circle", tint: "forest", label: "Descuento", credit: true },
  scholarship: { icon: "check-circle", tint: "forest", label: "Beca", credit: true },
  payment: { icon: "check-circle", tint: "forest", label: "Pago", credit: true },
  credit_note: { icon: "check-circle", tint: "forest", label: "Nota de crédito", credit: true },
};

const METHOD_LABEL: Record<string, string> = { cash: "Efectivo", card: "Tarjeta", transfer: "Transferencia", credit_note: "Nota de crédito" };

const PDF_MESSAGES = {
  network: "No pudimos descargar el estado de cuenta. Revisa tu conexión e inténtalo de nuevo.",
  server: "La escuela no pudo entregar el estado de cuenta en este momento. Inténtalo más tarde.",
  storage: "No pudimos guardar el estado de cuenta en el teléfono. Libera espacio e inténtalo de nuevo.",
  not_found: "El estado de cuenta no está disponible.",
  session: "Tu sesión terminó. Vuelve a iniciar sesión para descargar el estado de cuenta.",
};

/** The row's second line: what the line is, its day, and how money came. */
function subOf(m: FamilyMovement): string {
  const parts = [KIND_LOOK[m.kind]?.label ?? m.kind, formatDate(m.date)];
  const method = m.method ? (METHOD_LABEL[m.method] ?? m.method) : "";
  // A credit note's method is its kind (T8): said once, not twice.
  if (method !== parts[0]) parts.push(method);
  if (m.folio) parts.push(m.folio);
  return parts.filter(Boolean).join(" · ");
}

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
  // Newest first: the order a parent reads a bank app in. The balances are
  // finance's, per line, whatever the order they are shown in.
  const movements = [...(data?.movements ?? [])].reverse();

  return (
    <>
      <ScreenTitle>Cuenta</ScreenTitle>
      {error ? <StaleNotice /> : null}

      <Card style={[styles.balanceHero, { marginTop: 12 }]}>
        <Text style={[styles.cap, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>
          {hasToday ? "A pagar hoy" : settled ? "Saldo actual" : "Saldo por pagar"}
        </Text>
        <Text
          accessibilityLabel="Saldo"
          style={[styles.amount, { color: settled ? theme.colors.ok : theme.colors.text, fontFamily: theme.fonts.display }]}
        >
          {pesos(Math.max(heroCents, 0))}
        </Text>
        {hasToday && (
          <>
            <Text
              accessibilityLabel={balanceCents < 0 ? "Saldo a favor" : "Saldo pendiente"}
              style={[styles.year, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}
            >
              {pesos(Math.abs(balanceCents))}
            </Text>
            <Text
              accessibilityLabel="Detalle del saldo"
              style={[styles.detail, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}
            >
              {`${(data?.surchargeCents ?? 0) > 0 ? `Incluye ${pesos(data?.surchargeCents ?? 0)} de recargo. ` : ""}` +
                `${(data?.earlyDiscountCents ?? 0) > 0 ? `Ya con ${pesos(data?.earlyDiscountCents ?? 0)} de pronto pago. ` : ""}` +
                `Al ${data?.asOf ?? ""}.`}
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
      {movements.length === 0 ? (
        <EmptyState icon="receipt" title="Sin movimientos" subtitle="Aquí verás los cargos y pagos de tu cuenta." />
      ) : (
        <Card>
          {movements.map((m, i) => (
            <MovementRow key={`${m.kind}-${m.date}-${i}`} movement={m} divider={i > 0} />
          ))}
        </Card>
      )}

      <View style={styles.action}>
        <Button label="Descargar estado de cuenta" icon="download" onPress={pdf.start} accessibilityLabel="Descargar estado de cuenta" />
        {pdf.state.status === "downloading" ? (
          <Text accessibilityLiveRegion="polite" style={[styles.status, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
            Descargando…
          </Text>
        ) : null}
        {pdf.state.status === "failed" ? (
          <Text accessibilityRole="alert" style={[styles.status, { color: theme.colors.bad, fontFamily: theme.fonts.uiSemi }]}>
            {pdf.state.message}
          </Text>
        ) : null}
      </View>
    </>
  );
}

function MovementRow({ movement, divider }: { movement: FamilyMovement; divider: boolean }): ReactElement {
  const theme = useTheme();
  const look = KIND_LOOK[movement.kind] ?? KIND_LOOK.charge;
  const amount = look.credit ? -movement.amountCents : movement.amountCents;
  return (
    <View accessible accessibilityLabel={`Movimiento ${movement.description}`}>
      <ListRow divider={divider}>
        <TintIcon name={look.icon} tint={look.tint} />
        <View style={{ flex: 1 }}>
          <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{movement.description}</Text>
          <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{subOf(movement)}</Text>
        </View>
        <View style={styles.trail}>
          <Text style={[styles.amountVal, { color: look.credit ? theme.colors.ok : theme.colors.text, fontFamily: theme.fonts.display }]}>
            {pesos(amount)}
          </Text>
          <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{`Saldo ${pesos(movement.balanceCents)}`}</Text>
          {movement.accrued ? <Chip kind="warn">Acumulado a hoy</Chip> : null}
        </View>
      </ListRow>
    </View>
  );
}

const styles = StyleSheet.create({
  balanceHero: { padding: 22, alignItems: "center" },
  cardLink: { fontSize: 16, marginLeft: 12 },
  year: { fontSize: 13, marginTop: 2 },
  detail: { fontSize: 12, marginTop: 6, textAlign: "center" },
  cap: { fontSize: 13, fontWeight: "700", marginBottom: 4 },
  amount: { fontSize: 44, fontWeight: "800", letterSpacing: -1.5 },
  center: { alignItems: "center", marginTop: 12 },
  rowTitle: { fontSize: 15, fontWeight: "800" },
  rowSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
  trail: { alignItems: "flex-end", gap: 4 },
  amountVal: { fontSize: 16, fontWeight: "700", letterSpacing: -0.2 },
  action: { marginTop: 18, gap: 10 },
  status: { fontSize: 16, fontWeight: "600", textAlign: "center" },
});

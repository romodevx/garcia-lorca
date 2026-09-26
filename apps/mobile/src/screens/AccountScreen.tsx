import { useFlag } from "@garcia-lorca/ui/flags";
import { router } from "expo-router";
import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyAccount, FamilyCharge, FamilyPayment } from "../api";
import type { IconName } from "../icons";
import { useAppState } from "../state/AppState";
import { useChildAccount } from "../state/familyData";
import { useRefreshHandler } from "../components/refresh";
import { useTheme, type TintName } from "../theme";
import { Card, Chip, EmptyState, ListRow, ScreenTitle, SectionLabel, TintIcon } from "../ui";
import { AsyncErrorState, AsyncLoading, StaleNotice } from "./AsyncStates";
import { formatDate, pesos } from "./format";

const CHARGE_STATUS_LABEL: Record<string, string> = {
  pending: "Pendiente",
  partial: "Abono parcial",
  paid: "Pagado",
  overdue: "Vencido",
};

/** One row in the movements ledger, derived from a charge or a payment. */
interface Movement {
  key: string;
  icon: IconName;
  tint: TintName;
  title: string;
  sub: string;
  amountCents: number;
  /** A credit (payment) renders in green with a minus sign. */
  credit: boolean;
  chip?: string;
  /** Epoch ms used to sort the ledger newest-first. */
  sortAt: number;
}

function epoch(iso: string): number {
  const t = new Date(iso).getTime();
  return Number.isNaN(t) ? 0 : t;
}

/** Builds the newest-first ledger from the statement's charges and payments. */
function toLedger(account: FamilyAccount): Movement[] {
  const charges: Movement[] = account.charges.map((c: FamilyCharge) => ({
    key: `charge-${c.id}`,
    icon: "receipt",
    tint: "orange",
    title: c.description,
    sub: `${CHARGE_STATUS_LABEL[c.status] ?? c.status} · Vence ${formatDate(c.dueDate)}`,
    amountCents: c.amountCents - c.discountCents,
    credit: false,
    chip: c.remainingCents <= 0 ? "Pagado" : undefined,
    sortAt: epoch(c.dueDate),
  }));
  const payments: Movement[] = account.payments.map((p: FamilyPayment) => ({
    key: `payment-${p.id}`,
    icon: "check-circle",
    tint: "forest",
    title: "Pago recibido",
    sub: `Pago · ${formatDate(p.paidAt)}`,
    amountCents: p.amountCents,
    credit: true,
    sortAt: epoch(p.paidAt),
  }));
  return [...charges, ...payments].sort((a, b) => b.sortAt - a.sortAt);
}

/**
 * Cuenta/Saldo: the child's real finance statement from
 * `GET /api/v1/family/students/{id}/account` — the outstanding balance and the
 * charges/payments ledger. Read-only: there is no guardian payment endpoint yet,
 * so the screen never fabricates a settled state. Handles loading/error/empty.
 */
export function AccountScreen(): ReactElement {
  const { selectedChildId, childLoading, childError, refreshChildren } = useAppState();
  const { data, loading, error, reload } = useChildAccount(selectedChildId);
  useRefreshHandler(reload);
  const cardEnabled = useFlag("module_student_card");
  const theme = useTheme();

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

  // What the school is asking for TODAY leads; the year's balance sits beside it.
  // Both are true at once, and a parent shown only the year is told a number
  // nobody is asking them for — they are also the one person who cannot walk to
  // the office and ask which is which.
  //
  // An EMPTY asOf means the gateway is older than M18 and sent none of it.
  // Rendering the day's figure as $0.00 there would tell a family that owes
  // $32,000 that they are settled — the same fabricated "Al corriente" this
  // screen already refuses when the child bootstrap fails (bug_005). So the hero
  // falls back to the year, and says nothing about today.
  const hasToday = (data?.asOf ?? "") !== "";
  const heroCents = hasToday ? (data?.dueTodayCents ?? 0) : (data?.totalBalanceCents ?? 0);
  const settled = heroCents <= 0;
  const movements = data ? toLedger(data) : [];

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
              accessibilityLabel="Saldo del año"
              style={[styles.year, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}
            >
              {pesos(Math.max(data?.totalBalanceCents ?? 0, 0))}
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

      <SectionLabel>Movimientos recientes</SectionLabel>
      {movements.length === 0 ? (
        <EmptyState icon="receipt" title="Sin movimientos" subtitle="Aquí verás los cargos y pagos de tu cuenta." />
      ) : (
        <Card>
          {movements.map((m, i) => (
            <MovementRow key={m.key} movement={m} divider={i > 0} />
          ))}
        </Card>
      )}
    </>
  );
}

function MovementRow({ movement, divider }: { movement: Movement; divider: boolean }): ReactElement {
  const theme = useTheme();
  const amount = movement.credit ? -movement.amountCents : movement.amountCents;
  return (
    <ListRow divider={divider}>
      <TintIcon name={movement.icon} tint={movement.tint} />
      <View style={{ flex: 1 }}>
        <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{movement.title}</Text>
        <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{movement.sub}</Text>
      </View>
      <View style={styles.trail}>
        <Text style={[styles.amountVal, { color: movement.credit ? theme.colors.ok : theme.colors.text, fontFamily: theme.fonts.display }]}>
          {pesos(amount)}
        </Text>
        {movement.chip ? <Chip kind="ok">{movement.chip}</Chip> : null}
      </View>
    </ListRow>
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
});

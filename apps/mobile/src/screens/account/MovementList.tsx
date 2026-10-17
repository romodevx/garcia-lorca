import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyMovement, MovementKind } from "../../api";
import type { IconName } from "../../icons";
import { useTheme, type TintName } from "../../theme";
import { Card, Chip, ListRow, TintIcon } from "../../ui";
import { formatDate, pesos } from "../format";
import { PdfStatus } from "./PdfStatus";
import type { ReceiptStatus } from "./useReceiptDownload";

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
 * What a deposit paid, each charge with its part (ISSUE-T14-106); empty when
 * the line names a single charge already ("Pago · Inscripción").
 */
function appliedLineOf(m: FamilyMovement): string {
  const parts = m.appliedTo ?? [];
  if (parts.length < 2) return "";
  return `Pagó: ${parts.map((p) => `${p.description} ${pesos(p.amountCents)}`).join(" · ")}`;
}

interface MovementListProps {
  movements: readonly FamilyMovement[];
  /** Opens a payment's receipt; rows without a deposit are not offered one. */
  onOpenReceipt?: (movement: FamilyMovement) => void;
  /** The tapped receipt's download: shown under its own row. */
  receiptStatus?: ReceiptStatus | null;
}

/** A card of ledger lines, in the order given. */
export function MovementList({ movements, onOpenReceipt, receiptStatus }: MovementListProps): ReactElement {
  // A deposit paid in two tenders has two lines: its status sits under the first.
  const statusAt = receiptStatus ? movements.findIndex((m) => m.depositId === receiptStatus.depositId) : -1;
  return (
    <Card>
      {movements.map((m, i) => (
        <View key={`${m.kind}-${m.date}-${i}`}>
          <MovementRow
            movement={m}
            divider={i > 0}
            onOpenReceipt={onOpenReceipt !== undefined && m.depositId ? () => onOpenReceipt(m) : undefined}
          />
          {i === statusAt && receiptStatus && receiptStatus.state.status !== "idle" ? (
            <View testID={`receipt-status-${receiptStatus.depositId}`} style={styles.rowStatus}>
              <PdfStatus state={receiptStatus.state} />
            </View>
          ) : null}
        </View>
      ))}
    </Card>
  );
}

/**
 * One line: what it is and its amount. No running balance (ISSUE-T14-106): a
 * payment can precede its charge's due day, and the balance after it then
 * reads as credit the family does not have. The header carries the total.
 */
function MovementRow({ movement, divider, onOpenReceipt }: { movement: FamilyMovement; divider: boolean; onOpenReceipt?: () => void }): ReactElement {
  const theme = useTheme();
  const look = KIND_LOOK[movement.kind] ?? KIND_LOOK.charge;
  const amount = look.credit ? -movement.amountCents : movement.amountCents;
  const applied = appliedLineOf(movement);
  const label = `Movimiento ${movement.description}`;
  const content = (
    <>
      <TintIcon name={look.icon} tint={look.tint} />
      <View style={{ flex: 1 }}>
        <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{movement.description}</Text>
        <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{subOf(movement)}</Text>
        {applied !== "" ? (
          <Text style={[styles.rowSub, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>{applied}</Text>
        ) : null}
      </View>
      <View style={styles.trail}>
        <Text style={[styles.amountVal, { color: look.credit ? theme.colors.ok : theme.colors.text, fontFamily: theme.fonts.display }]}>
          {pesos(amount)}
        </Text>
        {movement.accrued ? <Chip kind="warn">Acumulado a hoy</Chip> : null}
        {onOpenReceipt !== undefined ? (
          <Text style={[styles.receiptCue, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>Comprobante</Text>
        ) : null}
      </View>
    </>
  );
  // A payment opens its receipt (ISSUE-T14-107): the whole row is the target.
  if (onOpenReceipt !== undefined) {
    return (
      <ListRow divider={divider} onPress={onOpenReceipt} accessibilityLabel={label} accessibilityHint="Abre el comprobante de pago">
        {content}
      </ListRow>
    );
  }
  return (
    <View accessible accessibilityLabel={label}>
      <ListRow divider={divider}>{content}</ListRow>
    </View>
  );
}

const styles = StyleSheet.create({
  rowTitle: { fontSize: 15, fontWeight: "800" },
  rowSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
  trail: { alignItems: "flex-end", gap: 4 },
  amountVal: { fontSize: 16, fontWeight: "700", letterSpacing: -0.2 },
  rowStatus: { paddingHorizontal: 16, paddingBottom: 12 },
  receiptCue: { fontSize: 13, fontWeight: "800", textDecorationLine: "underline" },
});

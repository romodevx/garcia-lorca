import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyMovement, MovementKind } from "../../api";
import type { IconName } from "../../icons";
import { useTheme, type TintName } from "../../theme";
import { Card, Chip, ListRow, TintIcon } from "../../ui";
import { formatDate, pesos } from "../format";

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

/** A card of ledger lines, in the order given. */
export function MovementList({ movements }: { movements: readonly FamilyMovement[] }): ReactElement {
  return (
    <Card>
      {movements.map((m, i) => (
        <MovementRow key={`${m.kind}-${m.date}-${i}`} movement={m} divider={i > 0} />
      ))}
    </Card>
  );
}

/**
 * One line: what it is and its amount. No running balance (ISSUE-T14-106): a
 * payment can precede its charge's due day, and the balance after it then
 * reads as credit the family does not have. The header carries the total.
 */
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
          {movement.accrued ? <Chip kind="warn">Acumulado a hoy</Chip> : null}
        </View>
      </ListRow>
    </View>
  );
}

const styles = StyleSheet.create({
  rowTitle: { fontSize: 15, fontWeight: "800" },
  rowSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
  trail: { alignItems: "flex-end", gap: 4 },
  amountVal: { fontSize: 16, fontWeight: "700", letterSpacing: -0.2 },
});

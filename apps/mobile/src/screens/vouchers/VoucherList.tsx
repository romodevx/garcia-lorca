import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyPaymentVoucher, PaymentVoucherStatus } from "../../api";
import type { ChipKind } from "../../data/mock";
import type { IconName } from "../../icons";
import { useTheme } from "../../theme";
import { Card, Chip, EmptyState } from "../../ui";
import { formatDayWithYear, pesos } from "../format";
import { schoolToday } from "./voucherInput";

interface StatusLook {
  label: string;
  spoken: string;
  kind: ChipKind;
  icon: IconName;
}

const STATUS: Record<PaymentVoucherStatus, StatusLook> = {
  pending: { label: "Pendiente", spoken: "pendiente", kind: "warn", icon: "clock" },
  applied: { label: "Aplicado", spoken: "aplicado", kind: "ok", icon: "check" },
  rejected: { label: "Rechazado", spoken: "rechazado", kind: "bad", icon: "x" },
};

/** "$2,500.00 · pagado el 20 oct 2026", from whatever the family said. */
function claimLine(v: FamilyPaymentVoucher): string {
  const parts = [
    v.claimedAmountCents !== undefined ? pesos(v.claimedAmountCents) : "",
    v.claimedPaidOn !== undefined ? `pagado el ${formatDayWithYear(v.claimedPaidOn)}` : "",
  ].filter((p) => p !== "");
  return parts.join(" · ");
}

/** What the office did with it: the receipt's folio, or why it was refused. */
function outcomeLine(v: FamilyPaymentVoucher): string {
  if (v.status === "applied" && v.depositFolio) return `Recibo ${v.depositFolio}`;
  if (v.status === "rejected" && v.rejectionReason) return `Motivo: ${v.rejectionReason}`;
  return "";
}

function VoucherRow({ voucher }: { voucher: FamilyPaymentVoucher }): ReactElement {
  const theme = useTheme();
  const look = STATUS[voucher.status];
  const sentOn = formatDayWithYear(schoolToday(new Date(voucher.createdAt)));
  const secondary = [styles.line, { color: theme.colors.text2, fontFamily: theme.fonts.ui }];
  const claim = claimLine(voucher);
  const outcome = outcomeLine(voucher);
  return (
    <Card style={styles.row} accessibilityLabel={`Comprobante ${look.spoken}`}>
      <View style={styles.head}>
        <Text style={[styles.sent, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{`Enviado el ${sentOn}`}</Text>
        <Chip kind={look.kind} icon={look.icon}>{look.label}</Chip>
      </View>
      {claim !== "" && <Text style={secondary}>{claim}</Text>}
      {outcome !== "" && <Text style={[secondary, { color: theme.colors.text }]}>{outcome}</Text>}
    </Card>
  );
}

/** "Mis comprobantes": what the family sent and what the office did, newest first. */
export function VoucherList({ vouchers }: { vouchers: FamilyPaymentVoucher[] }): ReactElement {
  if (vouchers.length === 0) {
    return <EmptyState icon="receipt" title="Aún no has enviado comprobantes" subtitle="Aquí verás si la escuela ya aplicó tu pago." />;
  }
  // The API answers oldest first; the family looks for the one it just sent.
  const newestFirst = [...vouchers].reverse();
  return (
    <View style={styles.list}>
      {newestFirst.map((v) => (
        <VoucherRow key={v.id} voucher={v} />
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  list: { gap: 10 },
  row: { padding: 16, gap: 6 },
  head: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: 8 },
  sent: { fontSize: 15, flexShrink: 1 },
  line: { fontSize: 14, lineHeight: 19 },
});

import { type ReactElement } from "react";
import { StyleSheet, Text } from "react-native";
import { useAppState } from "../state/AppState";
import { announceVoucherSent, useChildPaymentVouchers } from "../state/familyData";
import { useRefreshHandler } from "../components/refresh";
import { useTheme } from "../theme";
import { EmptyState, SectionLabel } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";
import { useVoucherForm } from "./vouchers/useVoucherForm";
import { VoucherForm } from "./vouchers/VoucherForm";
import { VoucherList } from "./vouchers/VoucherList";
import { schoolToday } from "./vouchers/voucherInput";

const NOT_FOUND = 404;

export interface VoucherScreenProps {
  /** The school's today, YYYY-MM-DD; tests pin it. */
  today?: string;
}

/**
 * "¿Ya pagaste? Sube tu comprobante" (ADR 0040): the family sends a photo or a
 * PDF of its transfer for the office to apply, and follows what happened to
 * each one. The amount and day only prefill what the office confirms.
 */
export function VoucherScreen({ today = schoolToday() }: VoucherScreenProps): ReactElement {
  const theme = useTheme();
  const { selectedChildId } = useAppState();
  const vouchers = useChildPaymentVouchers(selectedChildId);
  // A send recounts this list and Cuenta's alike.
  const form = useVoucherForm(selectedChildId, today, announceVoucherSent);
  useRefreshHandler(vouchers.reload);

  // The gateway's 404 is the boundary: the module is off for this family.
  if (vouchers.errorStatus === NOT_FOUND) {
    return <EmptyState icon="lock" title="No disponible" subtitle="El envío de comprobantes no está disponible por ahora." />;
  }

  return (
    <>
      <Text style={[styles.intro, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>
        Envía la foto o el PDF de tu transferencia. La escuela lo revisa y lo aplica a tu cuenta; el importe y la fecha
        sólo le ayudan a encontrarlo.
      </Text>
      <VoucherForm form={form} />

      <SectionLabel>Mis comprobantes</SectionLabel>
      {vouchers.data !== null ? (
        <VoucherList vouchers={vouchers.data} />
      ) : vouchers.error ? (
        <AsyncErrorState title="No pudimos cargar tus comprobantes" subtitle="Revisa tu conexión e inténtalo de nuevo." onRetry={vouchers.reload} />
      ) : (
        <AsyncLoading />
      )}
    </>
  );
}

const styles = StyleSheet.create({
  intro: { fontSize: 14, lineHeight: 20, marginTop: 4 },
});

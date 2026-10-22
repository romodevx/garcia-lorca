import { type ReactNode } from "react";
import { View } from "react-native";
import { DetailHeader } from "@/src/components/DetailHeader";
import { GatedScreen } from "@/src/components/GatedScreen";
import { VoucherScreen } from "@/src/screens/VoucherScreen";
import { useTheme } from "@/src/theme";

/** Subir comprobante (ADR 0040) — pushed from Cuenta when module_payment_vouchers is on. */
export default function ComprobanteRoute(): ReactNode {
  const theme = useTheme();
  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <DetailHeader title="Subir comprobante" />
      <GatedScreen flag="module_payment_vouchers">
        <VoucherScreen />
      </GatedScreen>
    </View>
  );
}

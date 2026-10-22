import { router } from "expo-router";
import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { useTheme } from "../../theme";
import { Card, ListRow, TintIcon } from "../../ui";

const TITLE = "¿Ya pagaste? Sube tu comprobante";

function pendingLine(pending: number): string {
  if (pending === 0) return "Envía la foto o el PDF de tu transferencia.";
  return pending === 1 ? "1 comprobante por revisar" : `${pending} comprobantes por revisar`;
}

/** Cuenta's way to the voucher screen (ADR 0040), right below "Cómo pagar". */
export function VoucherEntryCard({ pending }: { pending: number }): ReactElement {
  const theme = useTheme();
  return (
    <Card onPress={() => router.push("/comprobante")} accessibilityLabel={TITLE} style={styles.card}>
      <ListRow>
        <TintIcon name="receipt" tint="forest" />
        <View style={styles.text}>
          <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{TITLE}</Text>
          <Text style={[styles.sub, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>{pendingLine(pending)}</Text>
        </View>
      </ListRow>
    </Card>
  );
}

const styles = StyleSheet.create({
  card: { marginTop: 12 },
  text: { marginLeft: 12, flexShrink: 1, gap: 2 },
  title: { fontSize: 16 },
  sub: { fontSize: 13 },
});

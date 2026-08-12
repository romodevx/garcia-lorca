import { useState, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { balance, money } from "../data/mock";
import type { IconName } from "../icons";
import { useTheme } from "../theme";
import { BottomSheet, Button, TintIcon } from "../ui";
import { SelectableRow } from "./SelectableRow";

export interface PaySheetProps {
  visible: boolean;
  onClose: () => void;
  /** Called when the parent confirms the payment. */
  onConfirm: () => void;
}

interface PayMethod {
  id: string;
  icon: IconName;
  title: string;
  sub: string;
}

// TODO(pendiente-backend: tokenización de tarjeta no construida; pago real usa finance)
// — dato estático provisional. No borrar. Ver .claude/docs/PLAN-mock-data-audit.md #B12.
const METHODS: PayMethod[] = [
  { id: "card", icon: "credit-card", title: "Tarjeta", sub: "•••• 4242" },
  { id: "spei", icon: "arrow-up-right", title: "Transferencia SPEI", sub: "CLABE de la escuela" },
  { id: "oxxo", icon: "receipt", title: "Pago en OXXO", sub: "Referencia para tienda" },
];

/** Bottom sheet to pay tuition: shows the amount, pick a method, confirm. */
export function PaySheet({ visible, onClose, onConfirm }: PaySheetProps): ReactElement {
  const theme = useTheme();
  const [method, setMethod] = useState("card");

  return (
    <BottomSheet visible={visible} onClose={onClose}>
      <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        Pagar colegiatura
      </Text>
      <Text style={[styles.sub, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
        Colegiatura de junio · vence 15 jun
      </Text>

      <View style={styles.amountWrap}>
        <Text style={[styles.amount, { color: theme.colors.text, fontFamily: theme.fonts.display }]}>
          {money(balance.amount)}
        </Text>
      </View>

      <Text style={[styles.section, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>
        MÉTODO DE PAGO
      </Text>

      {METHODS.map((m) => (
        <SelectableRow
          key={m.id}
          selected={method === m.id}
          onPress={() => setMethod(m.id)}
          accessibilityLabel={m.title}
        >
          <TintIcon name={m.icon} tint="forest" size={40} />
          <View style={{ flex: 1 }}>
            <Text style={[styles.methodTitle, { color: theme.colors.text, fontFamily: theme.fonts.ui }]}>{m.title}</Text>
            <Text style={[styles.methodSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>{m.sub}</Text>
          </View>
        </SelectableRow>
      ))}

      <Button
        label={`Pagar ${money(balance.amount)}`}
        accessibilityLabel="Confirmar pago"
        icon="lock"
        onPress={onConfirm}
        style={{ marginTop: 20 }}
      />
      <Button label="Cancelar" variant="ghost" small onPress={onClose} style={{ marginTop: 10 }} />
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  title: { fontSize: 21, fontWeight: "800", letterSpacing: -0.4 },
  sub: { fontSize: 13.5, fontWeight: "600", marginTop: 4 },
  amountWrap: { alignItems: "center", marginTop: 18, marginBottom: 8 },
  amount: { fontSize: 40, fontWeight: "800", letterSpacing: -1.5 },
  section: { fontSize: 12.5, fontWeight: "800", letterSpacing: 0.4, marginTop: 8, marginBottom: 6 },
  methodTitle: { fontSize: 14.5, fontWeight: "700" },
  methodSub: { fontSize: 13, fontWeight: "600", marginTop: 2 },
});

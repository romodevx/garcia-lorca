import { useState, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { balance, money, type Movement } from "../data/mock";
import type { IconName } from "../icons";
import { PaySheet } from "../sheets/PaySheet";
import { useAppState } from "../state/AppState";
import { useToast } from "../state/Toast";
import { useTheme, type TintName } from "../theme";
import { Button, Card, Chip, ListRow, ScreenTitle, SectionLabel, StatusDot, TintIcon } from "../ui";

const KIND: Record<Movement["kind"], { icon: IconName; tint: TintName }> = {
  pago: { icon: "check-circle", tint: "forest" },
  cargo: { icon: "receipt", tint: "orange" },
  descuento: { icon: "sparkles", tint: "purple" },
};

/**
 * Cuenta/Saldo: a large balance with a traffic-light state, a legend, and the
 * recent movements ledger. "Pagar ahora" opens the pay sheet; confirming flips
 * the balance to settled (live, via app state) and toasts. The whole screen
 * re-renders into its paid state.
 */
export function AccountScreen(): ReactElement {
  const theme = useTheme();
  const { paid, pay } = useAppState();
  const { fireToast } = useToast();
  const [sheetOpen, setSheetOpen] = useState(false);

  const confirmPay = (): void => {
    pay();
    setSheetOpen(false);
    fireToast("¡Pago realizado con éxito!");
  };

  return (
    <>
      <ScreenTitle>Cuenta</ScreenTitle>

      <Card style={[styles.balanceHero, { marginTop: 12 }]}>
        <Text style={[styles.cap, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>
          {paid ? "Saldo actual" : "Saldo por pagar"}
        </Text>
        <Text
          accessibilityLabel="Saldo"
          style={[styles.amount, { color: paid ? theme.colors.ok : theme.colors.text, fontFamily: theme.fonts.display }]}
        >
          {paid ? money(0) : money(balance.amount)}
        </Text>
        <View style={styles.center}>
          {paid ? (
            <Chip kind="ok" icon="check">Al corriente · ¡Gracias!</Chip>
          ) : (
            <Chip kind="warn" icon="clock">{`${balance.label} · ${balance.due}`}</Chip>
          )}
        </View>
        {paid ? (
          <Button label="Solicitar factura (CFDI)" variant="ghost" small icon="document" style={{ marginTop: 18 }} />
        ) : (
          <>
            <Button
              label={`Pagar ${money(balance.amount)}`}
              icon="credit-card"
              onPress={() => setSheetOpen(true)}
              style={{ marginTop: 18 }}
            />
            <Button label="Solicitar factura (CFDI)" variant="ghost" small icon="document" style={{ marginTop: 10 }} />
          </>
        )}
      </Card>

      {!paid ? (
        <Card padded style={[styles.legendCard, { marginTop: 12 }]}>
          {(
            [
              [theme.colors.ok, "Al corriente"],
              [theme.palette.orange, "Por vencer"],
              [theme.colors.bad, "Vencido"],
            ] as const
          ).map(([c, l]) => (
            <View key={l} style={styles.legendItem}>
              <StatusDot color={c} size={11} />
              <Text style={[styles.legendText, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>{l}</Text>
            </View>
          ))}
        </Card>
      ) : null}

      <SectionLabel>Movimientos recientes</SectionLabel>
      <Card>
        {paid ? (
          <ListRow>
            <TintIcon name="check-circle" tint="forest" />
            <View style={{ flex: 1 }}>
              <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Pago recibido</Text>
              <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>Colegiatura junio · hoy</Text>
            </View>
            <View style={styles.trail}>
              <Text style={[styles.amountPos, { color: theme.colors.ok, fontFamily: theme.fonts.display }]}>
                {money(-balance.amount)}
              </Text>
              <Chip kind="ok">Pagado</Chip>
            </View>
          </ListRow>
        ) : null}
        {balance.movements.map((m, i) => (
          <MovementRow key={`${m.date}-${m.title}`} movement={m} divider={paid || i > 0} />
        ))}
      </Card>

      <PaySheet visible={sheetOpen} onClose={() => setSheetOpen(false)} onConfirm={confirmPay} />
    </>
  );
}

function MovementRow({ movement, divider }: { movement: Movement; divider: boolean }): ReactElement {
  const theme = useTheme();
  const k = KIND[movement.kind];
  const negative = movement.amount < 0;
  return (
    <ListRow divider={divider}>
      <TintIcon name={k.icon} tint={k.tint} />
      <View style={{ flex: 1 }}>
        <Text style={[styles.rowTitle, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{movement.title}</Text>
        <Text style={[styles.rowSub, { color: theme.colors.text3, fontFamily: theme.fonts.uiSemi }]}>
          {movement.sub} · {movement.date}
        </Text>
      </View>
      <View style={styles.trail}>
        <Text style={[styles.amountVal, { color: negative ? theme.colors.ok : theme.colors.text, fontFamily: theme.fonts.display }]}>
          {money(movement.amount)}
        </Text>
        {movement.paid ? <Chip kind="ok">Pagado</Chip> : null}
      </View>
    </ListRow>
  );
}

const styles = StyleSheet.create({
  balanceHero: { padding: 22, alignItems: "center" },
  cap: { fontSize: 13, fontWeight: "700", marginBottom: 4 },
  amount: { fontSize: 44, fontWeight: "800", letterSpacing: -1.5 },
  center: { alignItems: "center", marginTop: 12 },
  legendCard: { flexDirection: "row", justifyContent: "space-around" },
  legendItem: { flexDirection: "row", alignItems: "center", gap: 7 },
  legendText: { fontSize: 12.5, fontWeight: "700" },
  rowTitle: { fontSize: 15, fontWeight: "800" },
  rowSub: { fontSize: 12.5, fontWeight: "600", marginTop: 3 },
  trail: { alignItems: "flex-end", gap: 4 },
  amountVal: { fontSize: 16, fontWeight: "700", letterSpacing: -0.2 },
  amountPos: { fontSize: 16, fontWeight: "700", letterSpacing: -0.2 },
});

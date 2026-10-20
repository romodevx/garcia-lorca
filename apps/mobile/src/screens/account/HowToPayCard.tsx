import * as Clipboard from "expo-clipboard";
import { useState, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyPaymentInfo } from "../../api";
import { useTheme } from "../../theme";
import { Button, Card, TintIcon } from "../../ui";

type Copied = "clabe" | "reference" | null;

/**
 * A CLABE grouped as banks print it: bank (3) · branch (3) · account (11) ·
 * check digit (1), so a parent can read it back to a teller digit by digit.
 */
export function groupClabe(clabe: string): string {
  if (clabe.length !== 18) return clabe;
  return `${clabe.slice(0, 3)} ${clabe.slice(3, 6)} ${clabe.slice(6, 17)} ${clabe.slice(17)}`;
}

/**
 * "Cómo pagar por transferencia": the school's CLABE and the child's own bank
 * reference, each with a copy button. Nothing shows while the school has no
 * CLABE. Without a reference the CLABE still shows, with a warning to ask for
 * one first: a deposit without it is one the school cannot tell apart.
 */
export function HowToPayCard({ info }: { info: FamilyPaymentInfo }): ReactElement | null {
  const theme = useTheme();
  const [copied, setCopied] = useState<Copied>(null);
  if (info.clabe === "") return null;

  const copy = (what: Exclude<Copied, null>, value: string) => (): void => {
    void Clipboard.setStringAsync(value).then(() => setCopied(what));
  };
  const label = [styles.label, { color: theme.colors.text2, fontFamily: theme.fonts.ui }];
  const value = [styles.value, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }];

  return (
    <Card style={styles.card} accessibilityLabel="Cómo pagar por transferencia">
      <View style={styles.header}>
        <TintIcon name="credit-card" tint="forest" />
        <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>Cómo pagar por transferencia</Text>
      </View>

      {info.bank !== "" && (
        <View style={styles.row}>
          <Text style={label}>Banco</Text>
          <Text style={value}>{info.bank}</Text>
        </View>
      )}
      {info.beneficiary !== "" && (
        <View style={styles.row}>
          <Text style={label}>Beneficiario</Text>
          <Text style={value}>{info.beneficiary}</Text>
        </View>
      )}

      <View style={styles.row}>
        <Text style={label}>CLABE</Text>
        <Text selectable style={[value, styles.number]}>{groupClabe(info.clabe)}</Text>
        <Button small variant="outline" icon="document" label="Copiar CLABE" accessibilityLabel="Copiar CLABE" onPress={copy("clabe", info.clabe)} />
      </View>

      <View style={styles.row}>
        <Text style={label}>Referencia del alumno</Text>
        {info.reference !== "" ? (
          <>
            <Text selectable style={[value, styles.number]}>{info.reference}</Text>
            <Button small variant="outline" icon="document" label="Copiar referencia" accessibilityLabel="Copiar referencia" onPress={copy("reference", info.reference)} />
            <Text style={[styles.note, { color: theme.colors.text2, fontFamily: theme.fonts.ui }]}>
              Escríbela en el campo «referencia» o «concepto» de tu transferencia: así la escuela sabe de quién es el pago.
            </Text>
          </>
        ) : (
          <Text style={[styles.note, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>
            Pide tu referencia en la escuela antes de pagar: sin ella no podemos saber de quién es el depósito.
          </Text>
        )}
      </View>

      {copied !== null && (
        <Text accessibilityLiveRegion="polite" style={[styles.note, { color: theme.colors.ok, fontFamily: theme.fonts.uiExtra }]}>
          {copied === "clabe" ? "CLABE copiada" : "Referencia copiada"}
        </Text>
      )}
    </Card>
  );
}

const styles = StyleSheet.create({
  card: { marginTop: 12, padding: 18, gap: 14 },
  header: { flexDirection: "row", alignItems: "center", gap: 12 },
  title: { fontSize: 16, flexShrink: 1 },
  row: { gap: 6 },
  label: { fontSize: 13 },
  value: { fontSize: 16 },
  number: { fontSize: 20, letterSpacing: 0.5 },
  note: { fontSize: 13, lineHeight: 18 },
});

import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { useTheme } from "../../theme";
import { Button, Card, ListRow, TintIcon } from "../../ui";
import { VoucherField } from "./VoucherField";
import type { VoucherFormState } from "./useVoucherForm";

function ErrorSummary({ items }: { items: string[] }): ReactElement | null {
  const theme = useTheme();
  if (items.length === 0) return null;
  return (
    <View accessible accessibilityRole="alert" accessibilityLabel="Revisa el formulario" style={[styles.summary, { backgroundColor: theme.colors.badBg }]}>
      {items.map((item) => (
        <Text key={item} style={[styles.note, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>{`• ${item}`}</Text>
      ))}
    </View>
  );
}

function FilePicker({ form }: { form: VoucherFormState }): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.block}>
      <Text style={[styles.label, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>Foto o PDF del comprobante</Text>
      {form.file !== null && (
        <View accessible accessibilityLabel={`Archivo a enviar: ${form.file.name}`}>
          <ListRow>
            <TintIcon name="document" tint="forest" />
            <View style={styles.fileText}>
              <Text style={[styles.label, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>Archivo a enviar</Text>
              <Text numberOfLines={1} style={[styles.fileName, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{form.file.name}</Text>
            </View>
          </ListRow>
        </View>
      )}
      <Button small variant="outline" icon="plus" label="Tomar foto" onPress={() => void form.pick("camera")} />
      <Button small variant="outline" icon="inbox" label="Elegir de la galería" onPress={() => void form.pick("library")} />
      <Button small variant="outline" icon="document" label="Elegir PDF" onPress={() => void form.pick("pdf")} />
      {form.errors.file !== null && (
        <Text style={[styles.note, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>{form.errors.file}</Text>
      )}
    </View>
  );
}

/** The voucher form: what was paid (a file), and optionally how much and when. */
export function VoucherForm({ form }: { form: VoucherFormState }): ReactElement {
  const theme = useTheme();
  return (
    <Card style={styles.card}>
      <ErrorSummary items={form.summary} />
      <FilePicker form={form} />
      <VoucherField
        label="Importe pagado"
        hint="(opcional)"
        value={form.amountText}
        error={form.errors.amount}
        keyboardType="decimal-pad"
        onChangeText={form.changeAmount}
        onBlur={form.blurAmount}
      />
      <VoucherField
        label="Fecha de pago"
        hint="(opcional)"
        value={form.dayText}
        error={form.errors.day}
        keyboardType="numbers-and-punctuation"
        onChangeText={form.changeDay}
        onBlur={form.blurDay}
      />
      {form.sendError !== null && (
        <Text accessibilityLiveRegion="polite" style={[styles.note, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>
          {form.sendError}
        </Text>
      )}
      {form.sent !== null && (
        <Text accessibilityLiveRegion="polite" style={[styles.note, { color: theme.colors.ok, fontFamily: theme.fonts.uiExtra }]}>
          {form.sent.replayed
            ? "Ya habías enviado este comprobante; no lo mandamos de nuevo."
            : "Listo, enviamos tu comprobante. La escuela lo revisará y aquí verás cuando lo aplique."}
        </Text>
      )}
      {/* Never disabled (forms rule): a tap while sending is ignored by the hook. */}
      <Button
        label={form.busy ? "Enviando…" : "Enviar comprobante"}
        accessibilityLabel="Enviar comprobante"
        icon="arrow-up-right"
        onPress={() => void form.submit()}
      />
    </Card>
  );
}

const styles = StyleSheet.create({
  card: { marginTop: 12, padding: 18, gap: 16 },
  block: { gap: 8 },
  label: { fontSize: 12.5, fontWeight: "800", marginHorizontal: 4 },
  fileText: { marginLeft: 12, flexShrink: 1, gap: 2 },
  fileName: { fontSize: 15 },
  summary: { borderRadius: 12, padding: 12, gap: 4 },
  note: { fontSize: 14, lineHeight: 19 },
});

import { useEffect, useState, type ReactElement } from "react";
import { StyleSheet, Text } from "react-native";
import { reasons } from "../data/mock";
import { useTheme } from "../theme";
import { BottomSheet, Button } from "../ui";
import { SelectableRow } from "./SelectableRow";

export interface ExcuseSheetProps {
  visible: boolean;
  /** Human label of the day being justified (e.g. "jue 4 jun"). */
  dayLabel: string;
  onClose: () => void;
  /** Called when the parent confirms a reason. */
  onConfirm: () => void;
}

/** Bottom sheet to justify an absence: pick a reason, send the justificante. */
export function ExcuseSheet({ visible, dayLabel, onClose, onConfirm }: ExcuseSheetProps): ReactElement {
  const theme = useTheme();
  const [reason, setReason] = useState<string | null>(null);

  // Reset the chosen reason whenever the sheet is freshly opened.
  useEffect(() => {
    if (visible) setReason(null);
  }, [visible]);

  return (
    <BottomSheet visible={visible} onClose={onClose}>
      <Text style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        Justificar inasistencia
      </Text>
      <Text style={[styles.sub, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
        Falta del {dayLabel}. Elige el motivo y la escuela lo revisará.
      </Text>
      <Text style={[styles.section, { color: theme.colors.text3, fontFamily: theme.fonts.uiExtra }]}>MOTIVO</Text>

      {reasons.map((r) => (
        <SelectableRow
          key={r.id}
          selected={reason === r.id}
          onPress={() => setReason(r.id)}
          accessibilityLabel={r.label}
        >
          <Text style={[styles.reasonLabel, { color: theme.colors.text, fontFamily: theme.fonts.ui }]}>{r.label}</Text>
        </SelectableRow>
      ))}

      <Button
        label="Enviar justificante"
        icon="check"
        disabled={reason === null}
        onPress={onConfirm}
        style={{ marginTop: 20 }}
      />
      <Button label="Cancelar" variant="ghost" small onPress={onClose} style={{ marginTop: 10 }} />
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  title: { fontSize: 21, fontWeight: "800", letterSpacing: -0.4 },
  sub: { fontSize: 13.5, fontWeight: "600", marginTop: 4, lineHeight: 19 },
  section: { fontSize: 12.5, fontWeight: "800", letterSpacing: 0.4, marginTop: 16, marginBottom: 6 },
  reasonLabel: { fontSize: 14.5, fontWeight: "700" },
});

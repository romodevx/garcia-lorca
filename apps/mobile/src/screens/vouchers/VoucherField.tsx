import { type ReactElement } from "react";
import { StyleSheet, Text, TextInput, View, type KeyboardTypeOptions } from "react-native";
import { useTheme } from "../../theme";

export interface VoucherFieldProps {
  label: string;
  /** Shown after the label, e.g. "(opcional)". */
  hint?: string;
  value: string;
  error: string | null;
  keyboardType: KeyboardTypeOptions;
  onChangeText: (text: string) => void;
  onBlur: () => void;
}

/** One labelled field of the voucher form: label above, error below, read out with it. */
export function VoucherField({ label, hint, value, error, keyboardType, onChangeText, onBlur }: VoucherFieldProps): ReactElement {
  const theme = useTheme();
  return (
    <View style={styles.wrap}>
      <Text style={[styles.label, { color: theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>
        {hint ? `${label} ${hint}` : label}
      </Text>
      <TextInput
        accessibilityLabel={label}
        accessibilityHint={error ?? undefined}
        value={value}
        onChangeText={onChangeText}
        onBlur={onBlur}
        keyboardType={keyboardType}
        style={[
          styles.input,
          {
            backgroundColor: theme.colors.surface2,
            borderColor: error ? theme.colors.bad : "transparent",
            color: theme.colors.text,
            fontFamily: theme.fonts.uiSemi,
          },
        ]}
      />
      {error !== null && (
        <Text style={[styles.error, { color: theme.colors.bad, fontFamily: theme.fonts.uiExtra }]}>{error}</Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { gap: 7 },
  label: { fontSize: 12.5, fontWeight: "800", marginHorizontal: 4 },
  input: { height: 54, paddingHorizontal: 16, borderRadius: 14, borderWidth: 1.6, fontSize: 16, fontWeight: "600" },
  error: { fontSize: 13, marginHorizontal: 4 },
});

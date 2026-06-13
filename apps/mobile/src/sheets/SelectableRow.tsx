import { type ReactElement, type ReactNode } from "react";
import { Pressable, StyleSheet, View } from "react-native";
import { useTheme } from "../theme";

export interface SelectableRowProps {
  selected: boolean;
  onPress: () => void;
  children: ReactNode;
  accessibilityLabel: string;
}

/** A tappable radio row (used in the excuse + pay sheets). */
export function SelectableRow({
  selected,
  onPress,
  children,
  accessibilityLabel,
}: SelectableRowProps): ReactElement {
  const theme = useTheme();
  return (
    <Pressable
      accessibilityRole="radio"
      accessibilityState={{ selected }}
      accessibilityLabel={accessibilityLabel}
      onPress={onPress}
      style={[
        styles.row,
        {
          backgroundColor: selected ? theme.colors.primarySoft : theme.colors.surface2,
          borderColor: selected ? theme.colors.primary : "transparent",
        },
      ]}
    >
      {children}
      <View style={[styles.radio, { borderColor: selected ? theme.colors.primary : theme.colors.lineStrong }]}>
        {selected ? <View style={[styles.dot, { backgroundColor: theme.colors.primary }]} /> : null}
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    paddingVertical: 15,
    paddingHorizontal: 16,
    borderRadius: 18,
    borderWidth: 1.6,
    marginTop: 10,
  },
  radio: { width: 22, height: 22, borderRadius: 11, borderWidth: 2, alignItems: "center", justifyContent: "center", marginLeft: "auto" },
  dot: { width: 11, height: 11, borderRadius: 5.5 },
});

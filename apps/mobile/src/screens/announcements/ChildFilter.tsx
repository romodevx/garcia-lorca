import { type ReactElement } from "react";
import { Pressable, ScrollView, StyleSheet, Text } from "react-native";
import { useTheme } from "../../theme";

/** The filter value that shows every child's notices. */
export const ALL_CHILDREN = "all";

export interface ChildFilterOption {
  id: string;
  name: string;
}

export interface ChildFilterProps {
  options: readonly ChildFilterOption[];
  value: string;
  onChange: (value: string) => void;
}

/**
 * "Todos" plus one pill per child (ISSUE-CC-303). Scrolls sideways so a third
 * or fourth sibling never squeezes the names; every target is ≥ 44 px tall.
 */
export function ChildFilter({ options, value, onChange }: ChildFilterProps): ReactElement {
  const all: ChildFilterOption = { id: ALL_CHILDREN, name: "Todos" };
  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.track} accessibilityRole="tablist">
      {[all, ...options].map((opt) => (
        <FilterPill key={opt.id} label={opt.name} active={opt.id === value} onPress={() => onChange(opt.id)} />
      ))}
    </ScrollView>
  );
}

function FilterPill({ label, active, onPress }: { label: string; active: boolean; onPress: () => void }): ReactElement {
  const theme = useTheme();
  return (
    <Pressable
      accessibilityRole="tab"
      accessibilityLabel={label}
      accessibilityState={{ selected: active }}
      onPress={onPress}
      style={[
        styles.pill,
        {
          backgroundColor: active ? theme.colors.primarySoft : theme.colors.surface2,
          borderColor: active ? theme.colors.primary : "transparent",
        },
      ]}
    >
      <Text style={[styles.label, { color: active ? theme.colors.text : theme.colors.text2, fontFamily: theme.fonts.uiExtra }]}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  track: { gap: 8, paddingBottom: 12 },
  pill: { minHeight: 44, paddingHorizontal: 16, borderRadius: 999, borderWidth: 1.6, alignItems: "center", justifyContent: "center" },
  label: { fontSize: 15, fontWeight: "800" },
});

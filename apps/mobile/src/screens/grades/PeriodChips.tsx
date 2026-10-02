import { type ReactElement } from "react";
import { Pressable, ScrollView, StyleSheet, Text } from "react-native";
import type { EvaluationPeriod } from "../../api/grades";
import { useTheme } from "../../theme";

export interface PeriodChipsProps {
  periods: EvaluationPeriod[];
  selected: string;
  onSelect: (periodId: string) => void;
}

/** The grade's evaluation periods as a horizontal row of chips; the selected one is filled. */
export function PeriodChips({ periods, selected, onSelect }: PeriodChipsProps): ReactElement {
  const theme = useTheme();
  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.row} style={styles.scroll}>
      {periods.map((period) => {
        const isSelected = period.id === selected;
        return (
          <Pressable
            key={period.id}
            accessibilityRole="button"
            accessibilityLabel={period.name}
            accessibilityState={{ selected: isSelected }}
            onPress={() => onSelect(period.id)}
            style={[
              styles.chip,
              { backgroundColor: isSelected ? theme.colors.primary : theme.colors.surfaceSunken },
            ]}
          >
            <Text
              style={[
                styles.label,
                { color: isSelected ? theme.colors.primaryInk : theme.colors.text2, fontFamily: theme.fonts.uiExtra },
              ]}
            >
              {period.name}
            </Text>
          </Pressable>
        );
      })}
    </ScrollView>
  );
}

const MIN_TOUCH_TARGET = 44;

const styles = StyleSheet.create({
  scroll: { marginTop: 14, flexGrow: 0 },
  row: { gap: 8, paddingRight: 8 },
  chip: {
    minHeight: MIN_TOUCH_TARGET,
    minWidth: 60,
    paddingHorizontal: 16,
    borderRadius: 999,
    alignItems: "center",
    justifyContent: "center",
  },
  label: { fontSize: 16, fontWeight: "800" },
});

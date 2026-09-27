import { type ReactElement } from "react";
import { Pressable, ScrollView, StyleSheet, Text } from "react-native";
import { useTheme } from "../../theme";
import { monthLabel } from "../attendanceView";
import { shortMonthLabel } from "../gradesView";

export interface MonthChipsProps {
  months: string[];
  selected: string;
  onSelect: (month: string) => void;
}

/** The cycle's months as a horizontal row of chips; the selected one is filled. */
export function MonthChips({ months, selected, onSelect }: MonthChipsProps): ReactElement {
  const theme = useTheme();
  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.row} style={styles.scroll}>
      {months.map((month) => {
        const isSelected = month === selected;
        return (
          <Pressable
            key={month}
            accessibilityRole="button"
            accessibilityLabel={monthLabel(month)}
            accessibilityState={{ selected: isSelected }}
            onPress={() => onSelect(month)}
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
              {shortMonthLabel(month)}
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

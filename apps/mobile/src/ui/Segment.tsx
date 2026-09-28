import { type ReactElement } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { useTheme } from "../theme";

export interface SegmentOption<T extends string> {
  value: T;
  label: string;
}

export interface SegmentProps<T extends string> {
  options: ReadonlyArray<SegmentOption<T>>;
  value: T;
  onChange: (value: T) => void;
}

/** Pill-style segmented control (Moderno: rounded, active segment highlighted). */
export function Segment<T extends string>({ options, value, onChange }: SegmentProps<T>): ReactElement {
  const theme = useTheme();
  return (
    <View style={[styles.track, { backgroundColor: theme.colors.surface2 }]}>
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <Pressable
            key={opt.value}
            accessibilityRole="tab"
            accessibilityState={{ selected: active }}
            onPress={() => onChange(opt.value)}
            style={[
              styles.button,
              active
                ? {
                    backgroundColor:
                      theme.name === "dark" ? theme.colors.primarySoft : theme.colors.surface,
                    borderColor: theme.colors.line,
                    borderWidth: 1,
                  }
                : null,
            ]}
          >
            <Text
              style={[
                styles.label,
                {
                  color: active ? theme.colors.text : theme.colors.text2,
                  fontFamily: theme.fonts.uiExtra,
                },
              ]}
            >
              {opt.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  track: { flexDirection: "row", borderRadius: 999, padding: 4, gap: 4 },
  button: { flex: 1, minHeight: 44, paddingHorizontal: 8, borderRadius: 999, alignItems: "center", justifyContent: "center" },
  label: { fontSize: 15, fontWeight: "800", textAlign: "center" },
});

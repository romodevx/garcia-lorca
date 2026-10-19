import { type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import type { FamilyStudent } from "../api";
import { SelectableRow } from "../sheets/SelectableRow";
import { toChildView } from "../state/childView";
import { useTheme } from "../theme";
import { Avatar, BottomSheet, Button } from "../ui";

export interface ChildSheetProps {
  visible: boolean;
  linkedChildren: readonly FamilyStudent[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onClose: () => void;
}

/**
 * Picks whose school life the app shows (ISSUE-CC-303): one radio row per
 * linked child, the current one marked. Choosing closes the sheet; every
 * per-child screen reads the selected id, so they all follow.
 */
export function ChildSheet({ visible, linkedChildren, selectedId, onSelect, onClose }: ChildSheetProps): ReactElement {
  const theme = useTheme();
  return (
    <BottomSheet visible={visible} onClose={onClose}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
        ¿De quién quieres ver la información?
      </Text>
      {linkedChildren.map((student) => {
        const view = toChildView(student);
        return (
          <SelectableRow
            key={student.id}
            selected={student.id === selectedId}
            accessibilityLabel={`${view.headerName}, ${view.subtitle}`}
            onPress={() => {
              onSelect(student.id);
              onClose();
            }}
          >
            <Avatar initials={view.initials} size={40} />
            <View style={styles.text}>
              <Text style={[styles.name, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>{view.headerName}</Text>
              <Text style={[styles.sub, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>{view.subtitle}</Text>
            </View>
          </SelectableRow>
        );
      })}
      <Button label="Cerrar" variant="ghost" onPress={onClose} style={styles.close} />
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  title: { fontSize: 21, fontWeight: "800", marginBottom: 4 },
  text: { flexShrink: 1 },
  name: { fontSize: 17, fontWeight: "800" },
  sub: { fontSize: 15, fontWeight: "600", marginTop: 2 },
  close: { marginTop: 18 },
});

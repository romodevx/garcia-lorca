import { useFlag } from "@garcia-lorca/ui/flags";
import { router } from "expo-router";
import { type ReactElement } from "react";
import { Pressable, StyleSheet } from "react-native";
import { Icon } from "../icons";
import { useAppState } from "../state/AppState";
import { useTheme } from "../theme";

const FAB_SIZE = 56;
const FAB_ICON_SIZE = 26;
/** Clearance between the button and whatever it floats over (edge, tab bar). */
const FAB_MARGIN = 16;
/** Bottom room a scrolling body needs so its last row clears the button. */
export const FAB_CLEARANCE = FAB_SIZE + 2 * FAB_MARGIN;

interface CardFabProps {
  /** Distance from the bottom of the overlay to the top of the tab bar; 0 until measured. */
  bottom: number;
}

/** Whether the credencial button shows: the module is on and a child is selected. */
export function useCardFabVisible(): boolean {
  const cardEnabled = useFlag("module_student_card");
  const { selectedChildId } = useAppState();
  return cardEnabled && selectedChildId !== null;
}

/**
 * Floating shortcut to the selected child's credencial, over every tab: at the
 * school door a parent should not have to dig through Cuenta for the QR.
 * UX-gated by `module_student_card`; the gateway stays the security boundary.
 */
export function CardFab({ bottom }: CardFabProps): ReactElement | null {
  const theme = useTheme();
  const visible = useCardFabVisible();
  // Before the bar is measured the button would be drawn on top of it.
  if (!visible || bottom === 0) return null;

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel="Mostrar credencial"
      onPress={() => router.push("/credencial")}
      style={({ pressed }) => [
        styles.fab,
        { bottom: bottom + FAB_MARGIN, backgroundColor: pressed ? theme.colors.primaryPress : theme.colors.primary },
      ]}
    >
      {/* The same primary / onAccent pair as the primary Button. */}
      <Icon name="id-card" size={FAB_ICON_SIZE} stroke={2.2} color={theme.colors.onAccent} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  fab: {
    position: "absolute",
    right: FAB_MARGIN,
    width: FAB_SIZE,
    height: FAB_SIZE,
    borderRadius: FAB_SIZE / 2,
    alignItems: "center",
    justifyContent: "center",
    elevation: 6,
    shadowColor: "#000000",
    shadowOpacity: 0.18,
    shadowRadius: 8,
    shadowOffset: { width: 0, height: 4 },
  },
});

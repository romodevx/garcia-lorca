import { useEffect, useRef, type ReactElement, type ReactNode } from "react";
import {
  Animated,
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  View,
  useWindowDimensions,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTheme } from "../theme";

export interface BottomSheetProps {
  visible: boolean;
  onClose: () => void;
  children?: ReactNode;
  /** Accessible label for the dismiss scrim. */
  closeLabel?: string;
}

/**
 * Native bottom sheet built on RN Modal + Animated (no external bottom-sheet
 * dependency, no reanimated/gesture-handler babel wiring). Slides up over a
 * tap-to-dismiss scrim, shows a grab handle, and scrolls its content.
 *
 * ADR note: the prototype implied a gesture-draggable sheet; this faithful
 * recreation uses tap-scrim + button dismissal. A draggable handle (gorhom or
 * a PanResponder) can be layered later without changing this API.
 */
export function BottomSheet({
  visible,
  onClose,
  children,
  closeLabel = "Cerrar",
}: BottomSheetProps): ReactElement {
  const theme = useTheme();
  const insets = useSafeAreaInsets();
  const { height } = useWindowDimensions();
  const translateY = useRef(new Animated.Value(height)).current;
  const scrimOpacity = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    if (visible) {
      Animated.parallel([
        Animated.timing(translateY, { toValue: 0, duration: 300, useNativeDriver: true }),
        Animated.timing(scrimOpacity, { toValue: 1, duration: 250, useNativeDriver: true }),
      ]).start();
    } else {
      translateY.setValue(height);
      scrimOpacity.setValue(0);
    }
  }, [visible, translateY, scrimOpacity, height]);

  return (
    <Modal visible={visible} transparent animationType="none" onRequestClose={onClose}>
      <View style={styles.root}>
        <Animated.View style={[StyleSheet.absoluteFill, { opacity: scrimOpacity }]}>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={closeLabel}
            onPress={onClose}
            style={[StyleSheet.absoluteFill, { backgroundColor: theme.colors.scrim }]}
          />
        </Animated.View>
        <Animated.View
          style={[
            styles.sheet,
            {
              backgroundColor: theme.colors.surface,
              maxHeight: height * 0.86,
              paddingBottom: 26 + insets.bottom,
              transform: [{ translateY }],
            },
          ]}
        >
          <View style={[styles.grab, { backgroundColor: theme.colors.lineStrong }]} />
          <ScrollView showsVerticalScrollIndicator={false} keyboardShouldPersistTaps="handled">
            {children}
          </ScrollView>
        </Animated.View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, justifyContent: "flex-end" },
  sheet: {
    width: "100%",
    borderTopLeftRadius: 28,
    borderTopRightRadius: 28,
    paddingTop: 10,
    paddingHorizontal: 18,
  },
  grab: { width: 40, height: 5, borderRadius: 3, alignSelf: "center", marginTop: 2, marginBottom: 14 },
});

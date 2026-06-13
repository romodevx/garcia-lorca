import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Animated, StyleSheet, Text } from "react-native";
import { Icon, type IconName } from "../icons";
import { useTheme } from "../theme";

/**
 * Toast controller. A single host renders one transient toast at the bottom of
 * the screen; any component calls `fireToast()` to show it. Mirrors the
 * prototype's bottom toast (slide-up + fade).
 */
interface ToastValue {
  fireToast: (message: string, icon?: IconName) => void;
}

const ToastContext = createContext<ToastValue | null>(null);

const VISIBLE_MS = 2600;

interface ToastState {
  message: string;
  icon: IconName;
}

export function ToastProvider({ children }: { children: ReactNode }): ReactNode {
  const theme = useTheme();
  const [toast, setToast] = useState<ToastState | null>(null);
  const opacity = useRef(new Animated.Value(0)).current;
  const translateY = useRef(new Animated.Value(40)).current;
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fireToast = useCallback((message: string, icon: IconName = "check-circle") => {
    setToast({ message, icon });
  }, []);

  useEffect(() => {
    if (toast === null) return;
    Animated.parallel([
      Animated.timing(opacity, { toValue: 1, duration: 220, useNativeDriver: true }),
      Animated.timing(translateY, { toValue: 0, duration: 320, useNativeDriver: true }),
    ]).start();

    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      Animated.parallel([
        Animated.timing(opacity, { toValue: 0, duration: 220, useNativeDriver: true }),
        Animated.timing(translateY, { toValue: 40, duration: 320, useNativeDriver: true }),
      ]).start(() => setToast(null));
    }, VISIBLE_MS);

    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [toast, opacity, translateY]);

  return (
    <ToastContext.Provider value={{ fireToast }}>
      {children}
      {toast !== null ? (
        <Animated.View
          accessibilityLiveRegion="polite"
          pointerEvents="none"
          style={[
            styles.toast,
            { backgroundColor: theme.palette.forest, opacity, transform: [{ translateY }] },
          ]}
        >
          <Icon name={toast.icon} size={22} stroke={2.4} color="#FFFFFF" />
          <Text style={[styles.text, { fontFamily: theme.fonts.ui }]}>{toast.message}</Text>
        </Animated.View>
      ) : null}
    </ToastContext.Provider>
  );
}

export function useToast(): ToastValue {
  const ctx = useContext(ToastContext);
  if (ctx === null) {
    throw new Error("useToast must be used within a <ToastProvider>.");
  }
  return ctx;
}

const styles = StyleSheet.create({
  toast: {
    position: "absolute",
    left: 18,
    right: 18,
    bottom: 96,
    borderRadius: 14,
    paddingVertical: 14,
    paddingHorizontal: 16,
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    zIndex: 60,
    elevation: 8,
  },
  text: { color: "#FFFFFF", fontSize: 14, fontWeight: "700", flexShrink: 1 },
});

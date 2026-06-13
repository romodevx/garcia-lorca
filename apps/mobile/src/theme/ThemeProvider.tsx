import AsyncStorage from "@react-native-async-storage/async-storage";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  darkColors,
  fonts,
  lightColors,
  palette,
  radius,
  spacing,
  typeScale,
  type ThemeColors,
} from "./tokens";

export type ThemeName = "light" | "dark";

export interface Theme {
  name: ThemeName;
  colors: ThemeColors;
  palette: typeof palette;
  spacing: typeof spacing;
  radius: typeof radius;
  type: typeof typeScale;
  fonts: typeof fonts;
}

interface ThemeContextValue {
  theme: Theme;
  toggleTheme: () => void;
  /** True once the persisted preference has been read from storage. */
  ready: boolean;
}

const STORAGE_KEY = "gl_theme";

function buildTheme(name: ThemeName): Theme {
  return {
    name,
    colors: name === "dark" ? darkColors : lightColors,
    palette,
    spacing,
    radius,
    type: typeScale,
    fonts,
  };
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export interface ThemeProviderProps {
  children: ReactNode;
  /** Initial theme before storage resolves (and the default in tests). */
  initial?: ThemeName;
}

/**
 * Provides the typed theme object and persists the light/dark choice to
 * AsyncStorage so it survives app restarts.
 */
export function ThemeProvider({ children, initial = "light" }: ThemeProviderProps): ReactNode {
  const [name, setName] = useState<ThemeName>(initial);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let active = true;
    AsyncStorage.getItem(STORAGE_KEY)
      .then((stored) => {
        if (active && (stored === "light" || stored === "dark")) {
          setName(stored);
        }
      })
      .catch(() => {
        // Storage read failing is non-fatal; fall back to the initial theme.
      })
      .finally(() => {
        if (active) setReady(true);
      });
    return () => {
      active = false;
    };
  }, []);

  const toggleTheme = useCallback(() => {
    setName((prev) => {
      const next: ThemeName = prev === "light" ? "dark" : "light";
      AsyncStorage.setItem(STORAGE_KEY, next).catch(() => {
        // Persisting failure is non-fatal; the in-memory theme still flips.
      });
      return next;
    });
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({ theme: buildTheme(name), toggleTheme, ready }),
    [name, toggleTheme, ready],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

/** The one hook for reading the active theme. */
export function useTheme(): Theme {
  const ctx = useContext(ThemeContext);
  if (ctx === null) {
    throw new Error("useTheme must be used within a <ThemeProvider>.");
  }
  return ctx.theme;
}

/** Access the theme toggle + readiness flag. */
export function useThemeControls(): Pick<ThemeContextValue, "toggleTheme" | "ready"> {
  const ctx = useContext(ThemeContext);
  if (ctx === null) {
    throw new Error("useThemeControls must be used within a <ThemeProvider>.");
  }
  return { toggleTheme: ctx.toggleTheme, ready: ctx.ready };
}

import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import { type ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { demoFlags } from "./flags/bootstrap";
import i18n from "./i18n/i18n";
import { AppStateProvider } from "./state/AppState";
import { ToastProvider } from "./state/Toast";
import { ThemeProvider, type ThemeName } from "./theme";
import type { LevelId } from "./data/mock";

export interface ProvidersProps {
  children: ReactNode;
  /** Flag map (defaults to the demo seed; tests can inject their own). */
  flags?: FlagSet;
  initialTheme?: ThemeName;
  initialLevelId?: LevelId;
}

/**
 * Composes every app-wide provider in the correct order:
 * SafeArea → I18n → Theme → Flags → AppState → Toast.
 *
 * Reused by the Expo Router root layout and by tests, so a screen rendered in
 * isolation has the same context the real app gives it.
 */
export function Providers({
  children,
  flags = demoFlags,
  initialTheme = "light",
  initialLevelId = "primaria",
}: ProvidersProps): ReactNode {
  return (
    <SafeAreaProvider
      initialMetrics={{
        frame: { x: 0, y: 0, width: 0, height: 0 },
        insets: { top: 0, left: 0, right: 0, bottom: 0 },
      }}
    >
      <I18nextProvider i18n={i18n}>
        <ThemeProvider initial={initialTheme}>
          <FlagsProvider flags={flags}>
            <AppStateProvider initialLevelId={initialLevelId}>
              <ToastProvider>{children}</ToastProvider>
            </AppStateProvider>
          </FlagsProvider>
        </ThemeProvider>
      </I18nextProvider>
    </SafeAreaProvider>
  );
}

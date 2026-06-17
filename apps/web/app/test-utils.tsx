import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import { NextIntlClientProvider } from "next-intl";
import type { ReactElement, ReactNode } from "react";
import { buildMessages } from "./i18n/config";
import { ThemeProvider } from "./theme/ThemeProvider";

/**
 * Wraps a UI tree in the app providers (i18n + theme + flags) for component
 * tests, mirroring the real layout so a component that calls `useTranslations`
 * renders in isolation. Defaults to Spanish; pass `flags` to drive feature-gate
 * behavior under test.
 */
export function Providers({
  children,
  flags = {}
}: {
  children: ReactNode;
  flags?: FlagSet;
}): ReactElement {
  return (
    <NextIntlClientProvider locale="es" messages={buildMessages("es")}>
      <ThemeProvider>
        <FlagsProvider flags={flags}>{children}</FlagsProvider>
      </ThemeProvider>
    </NextIntlClientProvider>
  );
}

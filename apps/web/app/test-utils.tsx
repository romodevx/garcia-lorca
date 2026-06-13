import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import type { ReactElement, ReactNode } from "react";
import { ThemeProvider } from "./theme/ThemeProvider";

/**
 * Wraps a UI tree in the app providers (theme + flags) for component tests.
 * Pass `flags` to drive feature-gate behavior under test.
 */
export function Providers({
  children,
  flags = {}
}: {
  children: ReactNode;
  flags?: FlagSet;
}): ReactElement {
  return (
    <ThemeProvider>
      <FlagsProvider flags={flags}>{children}</FlagsProvider>
    </ThemeProvider>
  );
}

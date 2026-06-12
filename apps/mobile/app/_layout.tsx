import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import { Stack } from "expo-router";
import type { ReactNode } from "react";

/**
 * Root layout. Bootstraps the single FlagsProvider for the mobile surface.
 *
 * FASE 0 stub: flags start empty (everything OFF — flags default OFF). Once the
 * gateway is up, fetch GET `<EXPO_PUBLIC_API_URL>/features` on boot and pass the
 * resolved map here. Backend is the security boundary; this is UX-only.
 */
const bootstrapFlags: FlagSet = {};

export default function RootLayout(): ReactNode {
  return (
    <FlagsProvider flags={bootstrapFlags}>
      <Stack screenOptions={{ headerShown: false }} />
    </FlagsProvider>
  );
}

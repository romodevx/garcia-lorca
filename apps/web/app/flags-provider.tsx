"use client";

import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import type { ReactNode } from "react";

/**
 * App-level flag bootstrap.
 *
 * SECURITY BOUNDARY: this is UX-only. The authoritative enforcement of a
 * disabled module lives in the backend gateway (disabled module routes return
 * 404 / PermissionDenied). These flags only decide what navigation/surfaces the
 * frontend renders. Once the gateway is up, replace `DEMO_FLAGS` by fetching
 * GET `<NEXT_PUBLIC_API_URL>/features` on boot and passing the resolved map.
 *
 * PRESENTATION SEED: for this design demo we turn ON only the surfaces that are
 * actually implemented in the web build (dashboard is always reachable; the
 * finance surface is gated by `module_finance`). Everything else stays OFF and
 * falls through to the friendly ModulePlaceholder — flags default OFF.
 */
const DEMO_FLAGS: FlagSet = {
  module_finance: true
};

export function AppFlagsProvider({ children }: { children: ReactNode }): ReactNode {
  return <FlagsProvider flags={DEMO_FLAGS}>{children}</FlagsProvider>;
}

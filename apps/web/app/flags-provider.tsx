"use client";

import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import type { ReactNode } from "react";

/**
 * App-level flag bootstrap.
 *
 * FASE 0 stub: flags start empty (everything OFF — flags default OFF). Once the
 * gateway is up, fetch GET `<NEXT_PUBLIC_API_URL>/features` on boot and pass the
 * resolved map here as the single source of truth. The backend remains the
 * security boundary; this is UX-only.
 */
const bootstrapFlags: FlagSet = {};

export function AppFlagsProvider({ children }: { children: ReactNode }): ReactNode {
  return <FlagsProvider flags={bootstrapFlags}>{children}</FlagsProvider>;
}

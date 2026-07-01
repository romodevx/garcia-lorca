import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { FlagKey, FlagSet } from "./types";

/**
 * Single source of truth for feature flags on the client.
 *
 * SECURITY NOTE: this provider is UX-only. The authoritative enforcement of a
 * disabled module lives in the backend gateway (a disabled module's routes
 * return 404 / PermissionDenied). The frontend uses flags only to avoid
 * rendering navigation and chunks for modules the user cannot reach.
 *
 * Flags are fetched once on boot (gateway `/features`) and injected here.
 */
const FlagsContext = createContext<FlagSet | null>(null);

export interface FlagsProviderProps {
  /** Flag map resolved at boot from the gateway `/features` endpoint. */
  flags: FlagSet;
  children: ReactNode;
}

export function FlagsProvider({ flags, children }: FlagsProviderProps): ReactNode {
  // Freeze the reference so consumers only re-render when the map identity changes.
  const value = useMemo(() => flags, [flags]);
  return <FlagsContext.Provider value={value}>{children}</FlagsContext.Provider>;
}

/**
 * The ONE hook for reading a flag. Never scatter raw `flags[key]` reads or ad-hoc
 * conditionals across the app — always go through `useFlag`.
 *
 * Flags default OFF: an unknown/absent key returns false.
 */
export function useFlag(flag: FlagKey): boolean {
  const flags = useContext(FlagsContext);
  if (flags === null) {
    throw new Error("useFlag must be used within a <FlagsProvider>.");
  }
  return flags[flag] === true;
}

/**
 * Read the whole flag set (e.g. to assemble the command registry, which filters
 * many modules at once). Prefer `useFlag` for a single gate; use this only when
 * you genuinely need the full map.
 */
export function useFlags(): FlagSet {
  const flags = useContext(FlagsContext);
  if (flags === null) {
    throw new Error("useFlags must be used within a <FlagsProvider>.");
  }
  return flags;
}

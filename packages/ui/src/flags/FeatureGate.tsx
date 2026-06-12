import type { ReactNode } from "react";
import { useFlag } from "./FlagsProvider";
import type { FlagKey } from "./types";

export interface FeatureGateProps {
  /** Module flag key from FLAGS.md (e.g. "module_attendance"). */
  flag: FlagKey;
  /** Rendered when the flag is ON. */
  children: ReactNode;
  /** Optional content rendered when the flag is OFF (defaults to nothing). */
  fallback?: ReactNode;
}

/**
 * Declarative, UX-only gate. Renders `children` only when `flag` is enabled.
 *
 * This hides UI for disabled modules; it is NOT a security control. The backend
 * is the security boundary (disabled module routes return 404). Disabled module
 * route chunks should additionally be lazy-loaded so they are never downloaded.
 */
export function FeatureGate({ flag, children, fallback = null }: FeatureGateProps): ReactNode {
  return useFlag(flag) ? children : fallback;
}

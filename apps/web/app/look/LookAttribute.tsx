"use client";

import { useFlag, type FlagKey } from "@garcia-lorca/ui/flags";
import { useEffect } from "react";
import { applyLook } from "./look";

/**
 * The ONE place the skin reads its flag. The home variant reads it in its own
 * single place (home/homeVariant.ts), so splitting the flag in two is a
 * one-line change in either file.
 */
const SKIN_FLAG: FlagKey = "module_ui_refresh";

export type LookAttributeProps = {
  /**
   * True once /features answered. Before that the flags are the OFF fallback,
   * and acting on them would undo the pre-paint look and flash the old one.
   */
  resolved: boolean;
};

/** Keeps `html[data-look]` in step with the flag. Renders nothing. */
export function LookAttribute({ resolved }: LookAttributeProps): null {
  const refresh = useFlag(SKIN_FLAG);
  useEffect(() => {
    if (!resolved) return;
    applyLook(refresh);
  }, [resolved, refresh]);
  return null;
}

"use client";

import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import { useEffect, useState, type ReactNode } from "react";
import { API_URL, IS_DESIGN_DEMO } from "./api-url";
import { fetchFeatures } from "./features";

/**
 * App-level flag bootstrap.
 *
 * SECURITY BOUNDARY: this is UX-only. The authoritative enforcement of a
 * disabled module lives in the backend gateway (disabled module routes return
 * 404 / PermissionDenied). These flags only decide what navigation/surfaces the
 * frontend renders.
 *
 * Unless NEXT_PUBLIC_DESIGN_DEMO is on, the resolved flag map is fetched once on boot
 * from the gateway's authoritative `GET /api/v1/features`. Until it resolves — or
 * if the gateway is unreachable — we fall back to DEMO_FLAGS so the app still
 * renders instead of erroring.
 *
 * FLAGS DEFAULT OFF: with a gateway configured, that fallback must NOT switch a
 * module on, because the surfaces now talk to real endpoints — a flag the gateway
 * never granted would render a screen whose every read 404s. So DEMO_FLAGS only
 * lights modules up in the standalone design demo (no gateway at all); against a
 * real gateway every module stays OFF until `/features` says otherwise, and an
 * OFF module falls through to the friendly ModulePlaceholder.
 */
const DEMO_FLAGS: FlagSet = {
  module_finance: IS_DESIGN_DEMO
};

export function AppFlagsProvider({ children }: { children: ReactNode }): ReactNode {
  const [flags, setFlags] = useState<FlagSet>(DEMO_FLAGS);

  useEffect(() => {
    if (IS_DESIGN_DEMO) {
      return; // No gateway to ask — keep DEMO_FLAGS.
    }
    const controller = new AbortController();
    fetchFeatures(API_URL, controller.signal)
      .then(setFlags)
      .catch(() => {
        // Gateway unreachable — keep the fallback so the app still renders.
      });
    return () => controller.abort();
  }, []);

  return <FlagsProvider flags={flags}>{children}</FlagsProvider>;
}

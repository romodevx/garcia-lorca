"use client";

import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import { useEffect, useState, type ReactNode } from "react";
import { API_URL, IS_DESIGN_DEMO } from "./api-url";
import { fetchFeatures } from "./features";
import { LookAttribute } from "./look/LookAttribute";

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
 *
 * The app-wide look is synced here too (<LookAttribute/>, review E7): this
 * provider lives in the root layout, survives navigation and covers pages that
 * never mount AppShell. It acts only once the flags are `resolved`, so the
 * pre-paint look is not undone by the OFF fallback; if the gateway is
 * unreachable the remembered look stays.
 */
const DEMO_FLAGS: FlagSet = {
  module_finance: IS_DESIGN_DEMO
};

export function AppFlagsProvider({ children }: { children: ReactNode }): ReactNode {
  const [flags, setFlags] = useState<FlagSet>(DEMO_FLAGS);
  // In the design demo DEMO_FLAGS are the answer, not a placeholder.
  const [resolved, setResolved] = useState(IS_DESIGN_DEMO);

  useEffect(() => {
    if (IS_DESIGN_DEMO) {
      return; // No gateway to ask — keep DEMO_FLAGS.
    }
    const controller = new AbortController();
    fetchFeatures(API_URL, controller.signal)
      .then((granted) => {
        setFlags(granted);
        setResolved(true);
      })
      .catch(() => {
        // Gateway unreachable — keep the fallback so the app still renders.
      });
    return () => controller.abort();
  }, []);

  return (
    <FlagsProvider flags={flags}>
      <LookAttribute resolved={resolved} />
      {children}
    </FlagsProvider>
  );
}

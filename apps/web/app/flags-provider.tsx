"use client";

import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import { useEffect, useState, type ReactNode } from "react";
import { fetchFeatures } from "./features";

/**
 * App-level flag bootstrap.
 *
 * SECURITY BOUNDARY: this is UX-only. The authoritative enforcement of a
 * disabled module lives in the backend gateway (disabled module routes return
 * 404 / PermissionDenied). These flags only decide what navigation/surfaces the
 * frontend renders.
 *
 * When NEXT_PUBLIC_API_URL is set, the resolved flag map is fetched once on boot
 * from the gateway's authoritative `GET /api/v1/features`. Until it resolves (or
 * when no gateway is configured — e.g. the standalone design demo — or the fetch
 * fails), we fall back to DEMO_FLAGS so the app is always usable.
 *
 * DEMO_FLAGS turns ON only the surfaces implemented in this web build; everything
 * else stays OFF and falls through to the friendly ModulePlaceholder.
 */
const DEMO_FLAGS: FlagSet = {
  module_finance: true
};

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "";

export function AppFlagsProvider({ children }: { children: ReactNode }): ReactNode {
  const [flags, setFlags] = useState<FlagSet>(DEMO_FLAGS);

  useEffect(() => {
    if (API_URL === "") {
      return; // No gateway configured (design demo) — keep DEMO_FLAGS.
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

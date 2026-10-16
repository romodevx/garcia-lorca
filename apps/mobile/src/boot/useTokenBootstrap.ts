import { useEffect, useState } from "react";
import { loadTokens } from "../api/tokenStore";

/**
 * Hydrates the in-memory token cache from durable storage once on boot and gates
 * first paint until it settles.
 *
 * Ordering matters: the access token must be in memory BEFORE anything that reads
 * it runs — notably AppState's mount effect, which fetches the family inbox and
 * needs a Bearer header. Callers should render a splash until this returns true,
 * and only then mount the providers, mirroring `useAppFonts`.
 */
export function useTokenBootstrap(): boolean {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let active = true;
    void loadTokens().catch(() => null).finally(() => {
      if (active) {
        setReady(true);
      }
    });
    return () => {
      active = false;
    };
  }, []);

  return ready;
}

import type { FlagSet } from "@garcia-lorca/ui/flags";

/**
 * Fetches the resolved module-flag map from the gateway's authoritative
 * `/api/v1/features` endpoint (FLAGS.md: the server is the source of truth; the
 * frontend uses flags for UX only). Returns a FlagSet of the ENABLED modules.
 * Throws on a network error or non-2xx so the caller can fall back.
 */
const FEATURES_PATH = "/api/v1/features";

export async function fetchFeatures(baseURL: string, signal?: AbortSignal): Promise<FlagSet> {
  const url = `${baseURL.replace(/\/+$/, "")}${FEATURES_PATH}`;
  const res = await fetch(url, { signal });
  if (!res.ok) {
    throw new Error(`features: HTTP ${res.status}`);
  }
  const data = (await res.json()) as Record<string, boolean>;
  const flags: FlagSet = {};
  for (const [key, enabled] of Object.entries(data)) {
    // Keep only enabled module_* keys; absent/false is default-deny via useFlag.
    if (enabled && key.startsWith("module_")) {
      (flags as Record<string, boolean>)[key] = true;
    }
  }
  return flags;
}

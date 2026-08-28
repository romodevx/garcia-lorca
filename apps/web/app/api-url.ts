/**
 * The single resolved gateway base URL for the browser.
 *
 * This exists because the same expression was copy-pasted into 20 files with one
 * default and into flags-provider.tsx with a DIFFERENT one, and the divergence
 * was load-bearing: `?? ""` in the flags provider doubled as the "there is no
 * gateway, use mock data" switch, so the very value that means "same origin" in
 * production also meant "serve fake data". One place, one meaning.
 */

/** Exported for testing; prefer the resolved `API_URL` below. */
export function resolveApiUrl(raw: string | undefined, nodeEnv: string | undefined): string {
  if (raw !== undefined) {
    // "/" and "" both mean same-origin; normalising here keeps every caller's
    // `base + "/api/v1/..."` concatenation free of double slashes.
    return raw.replace(/\/+$/, "");
  }
  // Unset in a production bundle: same-origin, NEVER localhost. A production
  // build pointing at http://localhost:8080 sends every visitor's browser to
  // their own machine — the app is simply dead, with no server-side trace.
  return nodeEnv === "production" ? "" : "http://localhost:8080";
}

export const API_URL = resolveApiUrl(process.env.NEXT_PUBLIC_API_URL, process.env.NODE_ENV);

/**
 * The standalone design demo (no gateway at all, mock flags). Its OWN flag: it
 * used to be inferred from an empty API_URL, which collided with the same-origin
 * deployment and would have shown a school its own screens filled with fabricated
 * data that looked real.
 */
export const IS_DESIGN_DEMO = process.env.NEXT_PUBLIC_DESIGN_DEMO === "true";

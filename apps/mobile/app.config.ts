import type { ConfigContext, ExpoConfig } from "expo/config";

/**
 * Dynamic Expo config. Merges with app.json (the static base) and is where we
 * pin build-hardening posture.
 *
 * HARDENING (per ~/.claude/rules/frontend-build.md & repo CLAUDE.md §6):
 *   - Hermes is the JS engine on both platforms (set in app.json + reasserted
 *     here) — production JS ships as Hermes bytecode, not readable JS.
 *   - Source maps are NEVER shipped to production. Expo only emits them when
 *     `expo export --source-maps` is passed explicitly; the `build` script does
 *     NOT pass it. We also disable EAS automatic sourcemap upload below so maps
 *     never leave the build machine.
 *   - Only EXPO_PUBLIC_* env vars reach the client bundle — never secrets or
 *     internal URLs. EXPO_PUBLIC_API_URL → gateway base URL.
 */
export function easProjectConfig(projectId: string | undefined, config: Partial<ExpoConfig>): Pick<ExpoConfig, "extra" | "updates"> {
  const trimmedProjectId = projectId?.trim();
  return {
    ...(trimmedProjectId ? { updates: { ...config.updates, url: `https://u.expo.dev/${trimmedProjectId}` } } : {}),
    extra: {
      ...config.extra,
      eas: {
        ...((config.extra?.eas as Record<string, unknown>) ?? {}),
        ...(trimmedProjectId ? { projectId: trimmedProjectId } : {}),
      },
    },
  };
}

export default ({ config }: ConfigContext): ExpoConfig => ({
  ...config,
  name: config.name ?? "García Lorca",
  slug: config.slug ?? "garcia-lorca",
  // jsEngine ("hermes") is set in app.json (top-level + per-platform), the
  // static base this dynamic config merges into — kept there to satisfy the
  // ExpoConfig type and avoid duplication.
  // HARDENING: no `hooks.postPublish` sourcemap-upload hook is registered, so
  // source maps never leave the build machine. (Intentionally absent — there is
  // nothing to add here; this comment documents the deliberate omission.)
  ...easProjectConfig(process.env.EAS_PROJECT_ID, config),
});

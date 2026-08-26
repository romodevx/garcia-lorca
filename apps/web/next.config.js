// @ts-check

/**
 * García Lorca — web (Next.js App Router) build configuration.
 *
 * HARDENING (non-negotiable, per ~/.claude/rules/frontend-build.md &
 * repo CLAUDE.md §6):
 *   - Production source maps are NEVER shipped. `productionBrowserSourceMaps`
 *     defaults to false; we set it EXPLICITLY so the guarantee is auditable and
 *     cannot regress silently on a Next.js upgrade.
 *   - `console.*` (except error/warn) is stripped from production bundles.
 *   - Minification + tree-shaking are on by default in `next build`; no
 *     `devtool`/source-map opt-in is added anywhere.
 * The CI gate `scripts/check-no-sourcemaps.mjs` fails the build if any *.map
 * file or `sourceMappingURL` comment is found in the production output.
 */

// eslint-disable-next-line @typescript-eslint/no-require-imports -- CommonJS Next config
const createNextIntlPlugin = require("next-intl/plugin");
// next-intl request config: resolves locale (cookie) + messages for SSR.
const withNextIntl = createNextIntlPlugin("./app/i18n/request.ts");

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,

  // Lean, self-contained server output: Next traces only the files the server
  // actually needs into .next/standalone, for a small container image and a
  // faster Cloud Run cold start. See deploy/COST-AND-DEPLOY.md §C4.
  output: "standalone",

  // HARDENING: never emit browser source maps in production builds.
  productionBrowserSourceMaps: false,

  // HARDENING: strip console.* (keep error/warn) from production client bundles.
  compiler: {
    removeConsole:
      process.env.NODE_ENV === "production"
        ? { exclude: ["error", "warn"] }
        : false
  },

  transpilePackages: [
    "@garcia-lorca/ui",
    "@garcia-lorca/domain",
    "@garcia-lorca/i18n",
    "@garcia-lorca/api-client"
  ]
};

module.exports = withNextIntl(nextConfig);

#!/usr/bin/env node
// @ts-check
/**
 * CI HARDENING GATE — fail the build if production client artifacts leak source
 * maps. Enforces ~/.claude/rules/frontend-build.md & repo CLAUDE.md §6:
 * source maps are NEVER shipped to production.
 *
 * Fails (exit 1) if, in the BROWSER-SHIPPED artifacts, it finds either:
 *   - any `*.map` file, or
 *   - a `//# sourceMappingURL=` comment inside a shipped .js/.css asset.
 *
 * Scope is the CLIENT bundle only: for Next.js the browser-shipped assets live
 * in `.next/static` (server/SSR maps under `.next/server` are NEVER sent to a
 * client, so scanning them produces false positives). For a Vite/Webpack app
 * the dir is `dist`. The default below targets `.next/static`.
 *
 * Usage:  node scripts/check-no-sourcemaps.mjs [clientBuildDir]
 * Default: .next/static  (Next.js client artifacts). DevOps wires this into the
 * web app's CI job, after `next build`, before deploy.
 */
import { readdirSync, readFileSync, statSync, existsSync } from "node:fs";
import { join, extname } from "node:path";

const buildDir = process.argv[2] ?? ".next/static";

// Only assets that actually ship to the browser need scanning. .next/cache and
// server-only chunks are not deployed to clients.
const SCANNABLE_EXTENSIONS = new Set([".js", ".css", ".mjs"]);
const SOURCE_MAP_URL_RE = /sourceMappingURL=/;

/** @type {string[]} */
const violations = [];

/** @param {string} dir */
function walk(dir) {
  for (const entry of readdirSync(dir)) {
    if (entry === "cache") continue; // .next/cache is build-local, never shipped
    const full = join(dir, entry);
    const info = statSync(full);
    if (info.isDirectory()) {
      walk(full);
      continue;
    }
    if (extname(full) === ".map") {
      violations.push(`source map file: ${full}`);
      continue;
    }
    if (SCANNABLE_EXTENSIONS.has(extname(full))) {
      const contents = readFileSync(full, "utf8");
      if (SOURCE_MAP_URL_RE.test(contents)) {
        violations.push(`sourceMappingURL comment in: ${full}`);
      }
    }
  }
}

if (!existsSync(buildDir)) {
  console.error(`[check-no-sourcemaps] build dir not found: ${buildDir}`);
  console.error("Run the production build first (e.g. `next build`).");
  process.exit(1);
}

walk(buildDir);

if (violations.length > 0) {
  console.error("[check-no-sourcemaps] FAIL — source maps must never ship:");
  for (const v of violations) console.error(`  - ${v}`);
  process.exit(1);
}

console.log(`[check-no-sourcemaps] OK — no source maps under ${buildDir}.`);

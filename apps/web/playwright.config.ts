import { defineConfig, devices } from "@playwright/test";

/**
 * End-to-end tests against the RUNNING stack.
 *
 * These are deliberately not unit tests with a browser bolted on. Everything
 * below the browser is real: the Next.js app, the gateway, every Go service and
 * a live Postgres. That is the point — the bugs this suite exists to catch are
 * the ones that live between the layers, where each side is individually correct
 * and the two disagree.
 *
 * Run against the compose stack:
 *   docker compose up -d
 *   pnpm --filter @garcia-lorca/web e2e
 *
 * WEB_URL and API_URL point at the local ports (3002 / 8080 here, because 3000
 * and 5432 are taken on this machine); override them for another environment.
 */
const WEB_URL = process.env.E2E_WEB_URL ?? "http://localhost:3002";

export default defineConfig({
  testDir: "./e2e",
  // The suite tests configuring a school from nothing, so it starts from nothing.
  globalSetup: "./e2e/global-setup.ts",
  // A demo runs on one machine against one database. Parallel workers would race
  // each other creating the same school year, and a suite that fails on its own
  // concurrency teaches nothing.
  workers: 1,
  fullyParallel: false,
  // Locally a flake should be visible, not smoothed over. CI retries once,
  // because a genuinely flaky test failing a merge is worse than a slow one.
  retries: process.env.CI === "true" ? 1 : 0,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI === "true" ? [["github"], ["list"]] : [["list"]],
  use: {
    baseURL: WEB_URL,
    // Kept only for failures: a passing run leaves nothing behind, a failing one
    // leaves everything needed to see why without reproducing it.
    trace: "retain-on-failure",
    video: "retain-on-failure",
    screenshot: "only-on-failure",
    actionTimeout: 10_000,
  },
  projects: [
    // The from-zero suite and the guardrails. They own the database: global setup
    // empties it, so they never see the demo's data and the demo never sees
    // theirs.
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
      // The palette spec belongs to `demo`, and adding it there is not enough:
      // this project matches everything it does not ignore, so without the
      // third entry the palette ran here TOO — against the database global
      // setup had just truncated, which is the one place its record results
      // can never exist.
      testIgnore: [
        /.*\.mobile\.spec\.ts/,
        /demo-data\.spec\.ts/,
        /palette\.spec\.ts/,
        // Tienen su propio viewport y su propio proyecto.
        /responsive\.spec\.ts/,
        /drawer\.spec\.ts/,
      ],
    },
    // The storeroom is worked from a phone, standing between the shelves, so the
    // narrow layout is a real surface and gets a real run.
    { name: "mobile", use: { ...devices["Pixel 7"] }, testMatch: /.*\.mobile\.spec\.ts/ },
    // Tablet acostada y monitor grande: los dos extremos que el sistema debe
    // cubrir. Cada spec fija su propio viewport con test.use, así que aquí sólo
    // se declara el proyecto que los recoge.
    {
      name: "responsive",
      use: { ...devices["Desktop Chrome"] },
      testMatch: /(responsive|drawer)\.spec\.ts/,
    },
    // READ-ONLY, and run against a SEEDED database rather than an empty one. It
    // is the check you want an hour before a demo: not "can the system be built
    // from nothing" but "does the school we are about to show actually render".
    // It creates nothing, so it can run immediately before walking on stage.
    {
      name: "demo",
      use: { ...devices["Desktop Chrome"] },
      // The palette joins this project rather than the from-zero one: it
      // navigates and never mutates, so it is safe against the seeded database,
      // and its record results need a school that actually has pupils in it.
      testMatch: /(demo-data|palette)\.spec\.ts/,
    },
  ],
});

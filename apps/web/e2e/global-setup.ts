import { execFileSync } from "node:child_process";
import path from "node:path";

/**
 * Empties the demo database before the suite runs.
 *
 * This suite asserts the FROM-ZERO path — a school being configured from
 * nothing — so it has to start from nothing, and a suite that leaves its own
 * pupils, cycles and garments behind stops testing that path on its second run.
 * It also keeps the demo database in the state the demo needs it in.
 *
 * It refuses to run against anything but a local stack. Everything it does is a
 * TRUNCATE, and TRUNCATE against a staging URL somebody happened to export is
 * not a mistake worth being able to make.
 */
export default function globalSetup(): void {
  const api = process.env.API_URL ?? "http://localhost:8080";
  if (!/^https?:\/\/(localhost|127\.0\.0\.1)(:|\/|$)/.test(api)) {
    throw new Error(`refusing to reset a non-local database: API_URL=${api}`);
  }
  // The demo project reads a SEEDED database and must never wipe it. Anything
  // that sets this is saying "the data in there is the point".
  if (process.env.E2E_SKIP_RESET === "1") return;

  const repoRoot = path.resolve(__dirname, "../../..");
  execFileSync("sh", ["scripts/reset-demo-db.sh"], { cwd: repoRoot, stdio: "inherit" });
}

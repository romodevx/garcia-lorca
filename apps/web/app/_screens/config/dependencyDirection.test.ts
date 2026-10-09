import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/*
 * The setup view (module_config_setup) is a removable vertical slice: setup/
 * may depend on this folder, never the other way round. Deleting setup/ must
 * not break the old Catalog or any shared panel (code review S3). The only
 * door into setup/ is AppShell, which picks the view by flag.
 */

const ROOT = "app/_screens/config";
const SETUP = join(ROOT, "setup");

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return path === SETUP ? [] : sourceFiles(path);
    return /\.(ts|tsx)$/.test(name) ? [path] : [];
  });
}

describe("config ↔ setup dependency direction", () => {
  it("nothing outside setup/ imports from it", () => {
    const offenders = sourceFiles(ROOT).filter((file) => /from "[^"]*\/setup\//.test(readFileSync(file, "utf8")));
    expect(offenders).toEqual([]);
  });
});

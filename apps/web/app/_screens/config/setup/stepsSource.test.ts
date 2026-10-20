import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/*
 * PLAN-redesign-estructura B1: the setup view shows 8 or 7 steps
 * (module_school_structure), so a component that numbers steps or walks to a
 * neighbour reads the list from useSetupSteps() — importing the STEPS constant
 * would freeze it at eight. Only the places that DEFINE or DEFAULT the list may.
 */
const ALLOWED = new Set(["setupSteps.ts", "stepsContext.tsx", "useOpenStep.ts", "stepFixtures.ts"]);

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.(ts|tsx)$/.test(name) && !/\.test\.tsx?$/.test(name) ? [path] : [];
  });
}

describe("the setup step list", () => {
  it("is read through the context everywhere but where it is defined", () => {
    const offenders = sources(__dirname)
      .filter((path) => !ALLOWED.has(path.split("/").at(-1) ?? ""))
      .filter((path) => /\bSTEPS\b/.test(readFileSync(path, "utf8").replace(/STRUCTURE_STEPS/g, "")));
    expect(offenders).toEqual([]);
  });
});

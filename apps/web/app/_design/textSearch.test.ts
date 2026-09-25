import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import { matchesSearch } from "./textSearch";

describe("matchesSearch", () => {
  it.each([
    ["perez", "ANDRÉS PÉREZ GARCÍA"],
    ["andres", "ANDRÉS PÉREZ GARCÍA"],
    ["sofia", "SOFÍA LÓPEZ MARTÍNEZ"],
    ["nino", "EL NIÑO"],
    ["PÉREZ", "andres perez"],
    ["  maria   garcia ", "MARÍA  GARCÍA LÓPEZ"],
  ])("finds %j in %j, ignoring accents, case and spacing", (query, field) => {
    expect(matchesSearch(query, field)).toBe(true);
  });

  it("matches when any of the fields does", () => {
    expect(matchesSearch("pega19", "ANDRÉS PÉREZ", "PEGA190512HDFRRN05")).toBe(true);
  });

  it("an empty query matches everything", () => {
    expect(matchesSearch("   ", "anything")).toBe(true);
  });

  it("does not match what is not there", () => {
    expect(matchesSearch("gomez", "ANDRÉS PÉREZ GARCÍA", "PEGA190512HDFRRN05")).toBe(false);
  });
});

// The office types without accents. A search written as
// `.toLowerCase().includes(...)` misses PÉREZ for "perez" (QA 2026-09-25,
// ISSUE-006), so no screen may filter that way: they go through matchesSearch.
describe("every screen search ignores accents", () => {
  const screens = join(__dirname, "..", "_screens");
  const files = (dir: string): string[] =>
    readdirSync(dir).flatMap((name) => {
      const path = join(dir, name);
      if (statSync(path).isDirectory()) return files(path);
      return /\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name) ? [path] : [];
    });

  it("no screen filters with toLowerCase().includes", () => {
    const offenders = files(screens).filter((f) => /toLowerCase\(\)\.includes\(/.test(readFileSync(f, "utf8")));
    expect(offenders).toEqual([]);
  });
});

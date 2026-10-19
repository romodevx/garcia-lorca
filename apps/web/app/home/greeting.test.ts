import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import { parseStylesheet } from "../_design/testing/declaredStyle";
import { todayLabel } from "./greeting";

describe("todayLabel (CC-315)", () => {
  it("capitalizes the sentence, never the month: \"Sábado, 17 de octubre\"", () => {
    expect(todayLabel(new Date("2026-10-17T09:00:00"))).toBe("Sábado, 17 de octubre");
  });

  it("the date line is not capitalized word by word in CSS", () => {
    const rules = parseStylesheet(readFileSync("app/globals.css", "utf8")).filter((r) => r.selectors.includes(".home-date"));
    expect(rules.length).toBeGreaterThan(0);
    for (const r of rules) {
      expect(r.declarations.find((d) => d.prop === "text-transform")?.value).not.toBe("capitalize");
    }
  });
});

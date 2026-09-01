import { describe, expect, it } from "vitest";

import robots from "./robots";

describe("robots.txt", () => {
  it("disallows every crawler across the whole site", () => {
    const rules = robots().rules;
    const rule = Array.isArray(rules) ? rules[0] : rules;
    expect(rule?.userAgent).toBe("*");
    expect(rule?.disallow).toBe("/");
  });

  it("does not advertise a sitemap", () => {
    // A sitemap would hand a crawler the very URLs this file exists to hide.
    expect(robots().sitemap).toBeUndefined();
  });
});

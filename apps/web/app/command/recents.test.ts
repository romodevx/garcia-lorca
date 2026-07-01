import { describe, expect, it } from "vitest";

import { pushRecent } from "./recents";

describe("pushRecent", () => {
  it("prepends the newest id", () => {
    expect(pushRecent(["a", "b"], "c")).toEqual(["c", "a", "b"]);
  });

  it("de-duplicates (moves an existing id to the front)", () => {
    expect(pushRecent(["a", "b", "c"], "c")).toEqual(["c", "a", "b"]);
  });

  it("caps the list length", () => {
    expect(pushRecent(["a", "b", "c", "d", "e"], "f", 5)).toEqual(["f", "a", "b", "c", "d"]);
  });

  it("stores only ids (strings) — never record data", () => {
    const out = pushRecent([], "students.nav");
    expect(out).toEqual(["students.nav"]);
    expect(out.every((x) => typeof x === "string")).toBe(true);
  });
});

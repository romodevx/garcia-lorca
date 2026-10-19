import { describe, expect, it } from "vitest";

import { failureKind } from "./failureKind";

describe("failureKind", () => {
  it.each([
    [Object.assign(new Error("already cancelled"), { status: 409 }), "conflict"],
    [Object.assign(new Error("forbidden"), { status: 403 }), "forbidden"],
    [new TypeError("Failed to fetch"), "network"],
    [Object.assign(new Error("boom"), { status: 500 }), "other"],
    ["not an error", "other"],
  ])("%s → %s", (cause, kind) => {
    expect(failureKind(cause)).toBe(kind);
  });
});

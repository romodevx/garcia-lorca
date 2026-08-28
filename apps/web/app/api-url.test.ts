import { describe, expect, it } from "vitest";

import { resolveApiUrl } from "./api-url";

/**
 * Regression for the two silent failure paths /autoplan found in the deploy
 * review. Both looked identical to "working" and neither logged anything.
 */
describe("resolveApiUrl", () => {
  it("uses an explicit value verbatim", () => {
    expect(resolveApiUrl("https://api.example.mx", "production")).toBe("https://api.example.mx");
  });

  it("treats an empty value as same-origin", () => {
    // "" is the same-origin setting: app/auth/api.ts does
    // `baseURL.replace(/\/+$/, "") + path`, so "" yields "/api/v1/...".
    expect(resolveApiUrl("", "production")).toBe("");
  });

  it("collapses a bare slash to same-origin", () => {
    // "/" is what the Docker build passes, because a non-empty value survives
    // env plumbing that would drop an empty string.
    expect(resolveApiUrl("/", "production")).toBe("");
  });

  it("NEVER falls back to localhost in a production bundle", () => {
    // THE BUG: 20 files defaulted to http://localhost:8080. Shipped unset, every
    // visitor's browser dials its OWN machine, the app is dead, and there is not
    // one server-side log to explain it. Same-origin is the only defensible
    // production default.
    expect(resolveApiUrl(undefined, "production")).toBe("");
  });

  it("still defaults to the local gateway in development", () => {
    expect(resolveApiUrl(undefined, "development")).toBe("http://localhost:8080");
  });

  it("strips trailing slashes so callers never build a double slash", () => {
    expect(resolveApiUrl("https://api.example.mx/", "production")).toBe("https://api.example.mx");
  });
});

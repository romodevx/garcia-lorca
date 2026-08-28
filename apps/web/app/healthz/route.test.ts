import { describe, expect, it } from "vitest";

import { GET } from "./route";

/**
 * The shared Cloud Run module hardcodes a startup probe against /healthz
 * (deploy/terraform/modules/cloud_run/main.tf). Every Go service answers it;
 * the web app did not, so its revision would fail the probe five times and
 * Cloud Run would kill the container — with a green build and no application
 * error anywhere. This endpoint is what makes the web service deployable.
 */
describe("GET /healthz", () => {
  it("answers 200 so the Cloud Run startup probe passes", async () => {
    const res = await GET();
    expect(res.status).toBe(200);
  });

  it("does not touch the gateway or the database", async () => {
    // A liveness probe that depends on a downstream service turns one outage
    // into two: the probe fails, Cloud Run kills the container, and the web
    // app goes down because the gateway was slow. Keep it self-contained.
    const originalFetch = globalThis.fetch;
    let fetchCalled = false;
    globalThis.fetch = (() => {
      fetchCalled = true;
      return Promise.reject(new Error("healthz must not make network calls"));
    }) as typeof fetch;
    try {
      const res = await GET();
      expect(res.status).toBe(200);
      expect(fetchCalled).toBe(false);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

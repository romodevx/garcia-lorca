import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchFeatures } from "./features";

function mockFetch(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: status >= 200 && status < 300,
      status,
      json: async () => body,
    })),
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("fetchFeatures", () => {
  it("returns only the enabled module_* flags", async () => {
    mockFetch(200, {
      module_students: true,
      module_finance: true,
      module_billing: false,
      module_grades: false,
    });
    const flags = await fetchFeatures("http://gateway:8080");
    expect(flags.module_students).toBe(true);
    expect(flags.module_finance).toBe(true);
    // Disabled modules are absent (default-deny), not false-valued.
    expect("module_billing" in flags).toBe(false);
  });

  it("builds the URL from the base (trailing slashes trimmed)", async () => {
    const spy = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({}) }));
    vi.stubGlobal("fetch", spy);
    await fetchFeatures("http://gateway:8080/");
    expect(spy).toHaveBeenCalledWith("http://gateway:8080/api/v1/features", expect.any(Object));
  });

  it("throws on a non-2xx response", async () => {
    mockFetch(503, {});
    await expect(fetchFeatures("http://gateway:8080")).rejects.toThrow(/HTTP 503/);
  });
});

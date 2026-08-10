import { afterEach, describe, expect, it, vi } from "vitest";
import { authFetch } from "./authFetch";

function res(status: number): Response {
  return new Response(null, { status });
}

afterEach(() => vi.restoreAllMocks());

describe("authFetch", () => {
  it("passes a successful response through untouched", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(res(200));
    const out = await authFetch("http://localhost:8080/api/v1/students");
    expect(out.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("on 401 refreshes once and retries the original request", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(res(401)) // original → expired
      .mockResolvedValueOnce(res(200)) // refresh → ok
      .mockResolvedValueOnce(res(200)); // retry → ok
    const out = await authFetch("http://localhost:8080/api/v1/students");
    expect(out.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(String(fetchMock.mock.calls[1]?.[0])).toContain("/api/v1/auth/refresh");
  });

  it("returns the 401 when refresh fails (no session to recover)", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(res(401)) // original
      .mockResolvedValueOnce(res(401)); // refresh → fail
    const out = await authFetch("http://localhost:8080/api/v1/students");
    expect(out.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not try to refresh a 401 from the refresh endpoint itself", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(res(401));
    const out = await authFetch("http://localhost:8080/api/v1/auth/refresh", { method: "POST" });
    expect(out.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

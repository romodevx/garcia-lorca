import { afterEach, describe, expect, it, vi } from "vitest";
import { changePassword, login, logout, refresh } from "./api";

afterEach(() => vi.restoreAllMocks());

describe("login", () => {
  it("posts credentials and returns only expiresIn + mustChangePassword (no tokens)", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ expiresIn: 900, mustChangePassword: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const res = await login("http://gw", "owner@example.com", "pw");
    expect(res).toEqual({ expiresIn: 900, mustChangePassword: true });
    expect(res).not.toHaveProperty("accessToken");
    expect(res).not.toHaveProperty("refreshToken");
    expect(fetchMock).toHaveBeenCalledWith("http://gw/api/v1/auth/login", expect.objectContaining({ method: "POST", credentials: "include" }));
  });

  it("throws on 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));
    await expect(login("http://gw", "owner@example.com", "bad")).rejects.toThrow();
  });
});

describe("refresh", () => {
  it("posts with credentials and no body, returning the new access lifetime", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ expiresIn: 900 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const res = await refresh("http://gw");
    expect(res.expiresIn).toBe(900);
    expect(fetchMock).toHaveBeenCalledWith("http://gw/api/v1/auth/refresh", expect.objectContaining({ method: "POST", credentials: "include" }));
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.body).toBeUndefined();
  });

  it("throws on 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));
    await expect(refresh("http://gw")).rejects.toThrow();
  });
});

describe("changePassword", () => {
  it("posts current+new and resolves on 204", async () => {
    // A 204 response must have a null body per the Fetch spec (undici enforces it).
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(changePassword("http://gw", "old", "newlongpass")).resolves.toEqual({ sessionRenewed: false });
    expect(fetchMock).toHaveBeenCalledWith("http://gw/api/v1/auth/change-password", expect.objectContaining({ method: "POST", credentials: "include" }));
  });

  // The gateway re-logs the caller in with the new password and sets fresh
  // cookies (200). A 204 means the password changed but that re-login failed:
  // the old session is revoked and the caller must sign in again.
  it("reports a renewed session on 200 and a dead one on 204", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ expiresIn: 900, mustChangePassword: false }), { status: 200, headers: { "Content-Type": "application/json" } }),
    ));
    await expect(changePassword("http://gw", "old", "newlongpass")).resolves.toEqual({ sessionRenewed: true });
  });
});

describe("logout", () => {
  it("posts to the logout endpoint and resolves on 204", async () => {
    // A 204 response must have a null body per the Fetch spec (undici enforces it).
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(logout("http://gw")).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith("http://gw/api/v1/auth/logout", expect.objectContaining({ method: "POST", credentials: "include" }));
  });
});

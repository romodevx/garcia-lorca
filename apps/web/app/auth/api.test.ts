import { afterEach, describe, expect, it, vi } from "vitest";
import { changePassword, login, logout } from "./api";

afterEach(() => vi.restoreAllMocks());

describe("login", () => {
  it("posts credentials and returns mustChangePassword", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ accessToken: "a", refreshToken: "r", expiresIn: 900, mustChangePassword: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const res = await login("http://gw", "d@x", "p");
    expect(res.mustChangePassword).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith("http://gw/api/v1/auth/login", expect.objectContaining({ method: "POST", credentials: "include" }));
  });

  it("throws on 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));
    await expect(login("http://gw", "d@x", "bad")).rejects.toThrow();
  });
});

describe("changePassword", () => {
  it("posts current+new and resolves on 204", async () => {
    // A 204 response must have a null body per the Fetch spec (undici enforces it).
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(changePassword("http://gw", "old", "newlongpass")).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith("http://gw/api/v1/auth/change-password", expect.objectContaining({ method: "POST", credentials: "include" }));
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

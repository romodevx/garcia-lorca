import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createRole,
  createUser,
  linkTeacherUser,
  listRoles,
  listUsers,
  setRolePermissions,
  setUserRoles,
  setUserStatus,
} from "./api";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** Returns the [url, init] of the single fetch call the helper made. */
function lastCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

const sampleUser = {
  id: "u1",
  email: "director@example.com",
  displayName: "Directora",
  status: "active",
  mustChangePassword: true,
  roleIds: ["r1"],
};

const sampleRole = { id: "r1", name: "Coordinación", permissions: ["users:read", "users:write"] };

describe("createUser", () => {
  it("POSTs email/displayName/tempPassword/roleIds with credentials", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleUser, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createUser(BASE, {
      email: "director@example.com",
      displayName: "Directora",
      tempPassword: "init-pw-123",
      roleIds: ["r1"],
    });

    expect(res).toEqual(sampleUser);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/users");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({
      email: "director@example.com",
      displayName: "Directora",
      tempPassword: "init-pw-123",
      roleIds: ["r1"],
    });
  });

  it("throws on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 409 })));
    await expect(
      createUser(BASE, { email: "x@example.com", displayName: "X", tempPassword: "init-pw-123", roleIds: [] }),
    ).rejects.toThrow();
  });
});

describe("listUsers", () => {
  it("GETs with credentials and unwraps the {users} envelope", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ users: [sampleUser] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listUsers(BASE);

    expect(res).toEqual([sampleUser]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/users");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listUsers(BASE)).resolves.toEqual([]);
  });
});

describe("setUserStatus", () => {
  it("POSTs the status to the user's status endpoint", async () => {
    const suspended = { ...sampleUser, status: "suspended" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(suspended));
    vi.stubGlobal("fetch", fetchMock);

    const res = await setUserStatus(BASE, "u1", "suspended");

    expect(res).toEqual(suspended);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/users/u1/status");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ status: "suspended" });
  });
});

describe("setUserRoles", () => {
  it("POSTs the roleIds to the user's roles endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleUser));
    vi.stubGlobal("fetch", fetchMock);

    const res = await setUserRoles(BASE, "u1", ["r1", "r2"]);

    expect(res).toEqual(sampleUser);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/users/u1/roles");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ roleIds: ["r1", "r2"] });
  });
});

describe("createRole", () => {
  it("POSTs name/permissions with credentials", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleRole, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createRole(BASE, { name: "Coordinación", permissions: ["users:read", "users:write"] });

    expect(res).toEqual(sampleRole);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/roles");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ name: "Coordinación", permissions: ["users:read", "users:write"] });
  });
});

describe("listRoles", () => {
  it("GETs with credentials and unwraps the {roles} envelope", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ roles: [sampleRole] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listRoles(BASE);

    expect(res).toEqual([sampleRole]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/roles");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listRoles(BASE)).resolves.toEqual([]);
  });
});

describe("setRolePermissions", () => {
  it("POSTs the permissions to the role's permissions endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleRole));
    vi.stubGlobal("fetch", fetchMock);

    const res = await setRolePermissions(BASE, "r1", ["roles:read"]);

    expect(res).toEqual(sampleRole);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/roles/r1/permissions");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ permissions: ["roles:read"] });
  });
});

describe("linkTeacherUser", () => {
  it("POSTs the userId to the teacher link endpoint", async () => {
    const teacher = { id: "t1", firstName: "Ada", lastName: "Lovelace", email: "ada@example.com", userId: "u1" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(teacher));
    vi.stubGlobal("fetch", fetchMock);

    const res = await linkTeacherUser(BASE, "t1", "u1");

    expect(res).toEqual(teacher);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/teachers/t1/user");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ userId: "u1" });
  });

  it("sends an empty userId to unlink", async () => {
    const teacher = { id: "t1", firstName: "Ada", lastName: "Lovelace", email: "ada@example.com", userId: "" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(teacher));
    vi.stubGlobal("fetch", fetchMock);

    await linkTeacherUser(BASE, "t1", "");

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/teachers/t1/user");
    expect(JSON.parse(init.body as string)).toEqual({ userId: "" });
  });
});

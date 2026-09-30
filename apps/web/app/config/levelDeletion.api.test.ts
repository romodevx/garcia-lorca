import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfigError, deleteLevel, getLevelDeletionPreview, listLevelTrash, restoreLevel } from "./api";

const BASE = "http://gw/";

afterEach(() => vi.restoreAllMocks());

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function refusal(status: number, code: string, text = "no"): Response {
  return new Response(text, { status, headers: { "X-Error-Code": code } });
}

function call(mock: ReturnType<typeof vi.fn>): [string, RequestInit | undefined] {
  const c = mock.mock.calls[0] as [string, RequestInit | undefined];
  return [c[0], c[1]];
}

const COUNTS = {
  gradeLevels: 6, groups: 12, subjects: 40, terms: 3, teachingAssignments: 30, students: 120, enrollments: 120,
  attendanceMarks: 900, monthlyGrades: 2000, reportCards: 120, tuitionPlans: 1, uniformItems: 4, schoolEvents: 2,
};

describe("getLevelDeletionPreview", () => {
  it("GETs the preview of one level with credentials", async () => {
    const preview = { levelId: "l 1", levelName: "PRIMARIA", empty: false, counts: COUNTS, keeps: { charges: 10, payments: 8 }, confirmToken: "tok", expiresAt: "2026-09-28T18:00:00Z" };
    const fetchMock = vi.fn().mockResolvedValue(json(preview));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getLevelDeletionPreview(BASE, "l 1")).resolves.toEqual(preview);

    const [url, init] = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/levels/l%201/deletion-preview");
    expect(init?.credentials).toBe("include");
  });

  it("throws a ConfigError with the status on a refusal", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("no existe", { status: 404 })));

    const err = (await getLevelDeletionPreview(BASE, "l1").catch((e: unknown) => e)) as ConfigError;

    expect(err).toBeInstanceOf(ConfigError);
    expect(err.status).toBe(404);
  });
});

describe("deleteLevel", () => {
  it("POSTs the three confirmations and returns the batch", async () => {
    const done = { batchId: "b1", deletedAt: "2026-09-28T17:00:00Z", restorableUntil: "2026-10-05T17:00:00Z", counts: COUNTS };
    const fetchMock = vi.fn().mockResolvedValue(json(done));
    vi.stubGlobal("fetch", fetchMock);

    await expect(deleteLevel(BASE, "l1", { confirmName: "PRIMARIA", confirmToken: "tok", password: "pw" })).resolves.toEqual(done);

    const [url, init] = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/levels/l1/delete");
    expect(init?.method).toBe("POST");
    expect(init?.credentials).toBe("include");
    expect(JSON.parse(init?.body as string)).toEqual({ confirmName: "PRIMARIA", confirmToken: "tok", password: "pw" });
  });

  it.each([
    [409, "PREVIEW_STALE"],
    [400, "NAME_MISMATCH"],
    [401, "REAUTH_FAILED"],
  ])("surfaces %i %s as the error code", async (status, code) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(refusal(status, code)));

    const err = (await deleteLevel(BASE, "l1", { confirmName: "X", confirmToken: "t", password: "p" }).catch((e: unknown) => e)) as ConfigError;

    expect(err.status).toBe(status);
    expect(err.code).toBe(code);
  });
});

describe("listLevelTrash", () => {
  it("unwraps {items}", async () => {
    const item = { batchId: "b1", levelId: "l1", levelName: "PRIMARIA", deletedAt: "2026-09-28T17:00:00Z", restorableUntil: "2026-10-05T17:00:00Z", deletedByName: "Luis", counts: COUNTS, status: "active" };
    const fetchMock = vi.fn().mockResolvedValue(json({ items: [item] }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listLevelTrash(BASE)).resolves.toEqual([item]);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/config/level-trash");
  });

  it("reads a missing list as empty", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({})));
    await expect(listLevelTrash(BASE)).resolves.toEqual([]);
  });
});

describe("restoreLevel", () => {
  it("POSTs the batch and returns what came back and what did not", async () => {
    const out = { restored: { gradeLevels: 6 }, skipped: [{ kind: "enrollments", count: 3, reason: "los alumnos ya están en otro grupo" }] };
    const fetchMock = vi.fn().mockResolvedValue(json(out));
    vi.stubGlobal("fetch", fetchMock);

    await expect(restoreLevel(BASE, "b 1")).resolves.toEqual(out);

    const [url, init] = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/level-trash/b%201/restore");
    expect(init?.method).toBe("POST");
  });

  it("reads a missing skipped list as empty", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ restored: {} })));
    await expect(restoreLevel(BASE, "b1")).resolves.toEqual({ restored: {}, skipped: [] });
  });

  it.each([
    [409, "LEVEL_NAME_TAKEN"],
    [410, "TRASH_EXPIRED"],
  ])("surfaces %i %s", async (status, code) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(refusal(status, code)));
    const err = (await restoreLevel(BASE, "b1").catch((e: unknown) => e)) as ConfigError;
    expect(err.code).toBe(code);
  });
});

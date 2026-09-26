import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfigError } from "../config/api";
import {
  assignTeachingLoad,
  createTeacher,
  deleteTeachingAssignment,
  reassignTeachingAssignment,
  createTeachingAssignment,
  listTeachers,
  listTeachingAssignments,
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

describe("teachers", () => {
  it("createTeacher POSTs firstName/lastName/email with credentials", async () => {
    const created = { id: "tch1", firstName: "Ada", lastName: "Lovelace", email: "ada@example.com", userId: "" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createTeacher(BASE, { firstName: "Ada", lastName: "Lovelace", email: "ada@example.com" });

    expect(res).toEqual(created);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teachers");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ firstName: "Ada", lastName: "Lovelace", email: "ada@example.com" });
  });

  it("listTeachers GETs with credentials and unwraps the {teachers} envelope", async () => {
    const teachers = [{ id: "tch1", firstName: "Ada", lastName: "Lovelace", email: "ada@example.com", userId: "" }];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ teachers }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listTeachers(BASE);

    expect(res).toEqual(teachers);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teachers");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("listTeachers returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listTeachers(BASE)).resolves.toEqual([]);
  });
});

describe("teaching assignments", () => {
  it("createTeachingAssignment POSTs groupId/subjectId/teacherId and NEVER sends cycleId", async () => {
    const created = { id: "ta1", cycleId: "c1", groupId: "grp1", subjectId: "s1", teacherId: "tch1" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createTeachingAssignment(BASE, { groupId: "grp1", subjectId: "s1", teacherId: "tch1" });

    expect(res).toEqual(created);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching-assignments");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ groupId: "grp1", subjectId: "s1", teacherId: "tch1" });
    expect(body).not.toHaveProperty("cycleId");
  });

  it("listTeachingAssignments unwraps the {teachingAssignments} envelope", async () => {
    const teachingAssignments = [{ id: "ta1", cycleId: "c1", groupId: "grp1", subjectId: "s1", teacherId: "tch1" }];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ teachingAssignments }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listTeachingAssignments(BASE)).resolves.toEqual(teachingAssignments);
  });

  it("listTeachingAssignments applies the given filters as query params", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ teachingAssignments: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listTeachingAssignments(BASE, { cycleId: "c1", groupId: "grp1", teacherId: "tch1" });
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/teaching-assignments?cycleId=c1&groupId=grp1&teacherId=tch1");
  });

  it("listTeachingAssignments drops empty filters (only groupId given)", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ teachingAssignments: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listTeachingAssignments(BASE, { groupId: "grp1" });
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/teaching-assignments?groupId=grp1");
  });

  it("listTeachingAssignments sends no query string when no filters are given", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ teachingAssignments: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listTeachingAssignments(BASE);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/teaching-assignments");
  });
});

describe("carga académica", () => {
  const ta = { id: "ta1", cycleId: "c1", groupId: "g1", subjectId: "s1", teacherId: "t1" };

  it("assignTeachingLoad POSTs teacherId, items and replaceConflicts to /bulk and returns every list", async () => {
    const result = {
      created: [ta],
      reassigned: [],
      skipped: [{ groupId: "g2", subjectId: "s1" }],
      conflicts: [{ groupId: "g3", subjectId: "s1", assignmentId: "ta9", teacherId: "t9", teacherName: "Ana Ruiz" }],
    };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(result));
    vi.stubGlobal("fetch", fetchMock);

    const res = await assignTeachingLoad(BASE, {
      teacherId: "t1",
      items: [{ groupId: "g1", subjectId: "s1" }],
      replaceConflicts: false,
    });

    expect(res).toEqual(result);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching-assignments/bulk");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({
      teacherId: "t1",
      items: [{ groupId: "g1", subjectId: "s1" }],
      replaceConflicts: false,
    });
  });

  it("assignTeachingLoad fills a missing list with [] so callers never guard", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ created: [ta] })));
    await expect(
      assignTeachingLoad(BASE, { teacherId: "t1", items: [{ groupId: "g1", subjectId: "s1" }], replaceConflicts: false }),
    ).resolves.toEqual({ created: [ta], reassigned: [], skipped: [], conflicts: [] });
  });

  it("assignTeachingLoad throws a ConfigError carrying the status", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("items[0]: unknown group", { status: 400 })));
    const call = assignTeachingLoad(BASE, { teacherId: "t1", items: [{ groupId: "x", subjectId: "s1" }], replaceConflicts: false });
    await expect(call).rejects.toBeInstanceOf(ConfigError);
    await expect(call).rejects.toMatchObject({ status: 400 });
  });

  it("deleteTeachingAssignment DELETEs the assignment and resolves on 204", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(deleteTeachingAssignment(BASE, "ta 1")).resolves.toBeUndefined();

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching-assignments/ta%201");
    expect(init.method).toBe("DELETE");
    expect(init.credentials).toBe("include");
  });

  it("deleteTeachingAssignment throws on 404", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 404 })));
    await expect(deleteTeachingAssignment(BASE, "ta1")).rejects.toMatchObject({ status: 404 });
  });

  it("reassignTeachingAssignment PATCHes the new teacherId and returns the assignment", async () => {
    const moved = { ...ta, teacherId: "t2" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(moved));
    vi.stubGlobal("fetch", fetchMock);

    await expect(reassignTeachingAssignment(BASE, "ta1", "t2")).resolves.toEqual(moved);

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching-assignments/ta1");
    expect(init.method).toBe("PATCH");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ teacherId: "t2" });
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createTeacher,
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

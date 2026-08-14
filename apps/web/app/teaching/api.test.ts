import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { captureGrade, getRoster, listAssignments, listTerms } from "./api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
});

beforeEach(() => {
  // The client reads its base URL from the public env at call time.
  vi.stubEnv("NEXT_PUBLIC_API_URL", "http://gw");
});

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

const assignment = {
  id: "a1",
  cycleId: "c1",
  groupId: "g1",
  subjectId: "s1",
  teacherId: "t1",
  groupName: "1° A",
  subjectName: "Matemáticas",
};

const term = { id: "term1", name: "Bloque 1", cycleId: "c1" };

const rosterRow = {
  enrollmentId: "en1",
  studentId: "st1",
  studentName: "Ada Lovelace",
  value: 9.5,
  hasValue: true,
};

const gradeEntry = {
  id: "ge1",
  enrollmentId: "en1",
  subjectId: "s1",
  termId: "term1",
  teacherId: "t1",
  value: 9.5,
};

describe("listAssignments", () => {
  it("GETs /teaching/assignments with credentials and unwraps {assignments}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ assignments: [assignment] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listAssignments();

    expect(res).toEqual([assignment]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching/assignments");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listAssignments()).resolves.toEqual([]);
  });

  it("throws on a non-ok response (e.g. teacher lacks the flag → 404)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 404 })));
    await expect(listAssignments()).rejects.toThrow();
  });
});

describe("listTerms", () => {
  it("GETs /teaching/terms with credentials and unwraps {terms}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ terms: [term] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listTerms();

    expect(res).toEqual([term]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching/terms");
    expect(init.credentials).toBe("include");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listTerms()).resolves.toEqual([]);
  });
});

describe("getRoster", () => {
  it("GETs /teaching/roster with credentials, the query params, and unwraps {roster}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ roster: [rosterRow] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await getRoster("a1", "term1");

    expect(res).toEqual([rosterRow]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching/roster?assignmentId=a1&termId=term1");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("throws on a non-ok response (e.g. 403)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 403 })));
    await expect(getRoster("a1", "term1")).rejects.toThrow();
  });
});

describe("captureGrade", () => {
  it("POSTs {assignmentId, enrollmentId, termId, value} and NEVER sends subject/teacher", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(gradeEntry, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await captureGrade({ assignmentId: "a1", enrollmentId: "en1", termId: "term1", value: 9.5 });

    expect(res).toEqual(gradeEntry);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching/grades");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ assignmentId: "a1", enrollmentId: "en1", termId: "term1", value: 9.5 });
    // subject/teacher are DERIVED server-side — the client must never send them.
    expect(body).not.toHaveProperty("subjectId");
    expect(body).not.toHaveProperty("teacherId");
    expect(typeof body.value).toBe("number");
  });

  it("throws on a non-ok response (e.g. server range 400)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 400 })));
    await expect(
      captureGrade({ assignmentId: "a1", enrollmentId: "en1", termId: "term1", value: 11 }),
    ).rejects.toThrow();
  });
});

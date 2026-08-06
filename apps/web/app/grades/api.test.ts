import { afterEach, describe, expect, it, vi } from "vitest";
import { captureGrade, getRoster, listGradeEntries } from "./api";

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

const gradeEntry = {
  id: "ge1",
  enrollmentId: "en1",
  subjectId: "sub1",
  termId: "t1",
  teacherId: "tch1",
  value: 9.5,
};

const rosterRow = {
  enrollmentId: "en1",
  studentId: "st1",
  value: 9.5,
  hasValue: true,
};

describe("captureGrade", () => {
  it("POSTs {assignmentId, enrollmentId, termId, value} and NEVER sends subject/teacher", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(gradeEntry, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await captureGrade(BASE, {
      assignmentId: "asg1",
      enrollmentId: "en1",
      termId: "t1",
      value: 9.5,
    });

    expect(res).toEqual(gradeEntry);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ assignmentId: "asg1", enrollmentId: "en1", termId: "t1", value: 9.5 });
    // subject/teacher are DERIVED server-side — the client must never send them.
    expect(body).not.toHaveProperty("subjectId");
    expect(body).not.toHaveProperty("teacherId");
    expect(body).not.toHaveProperty("cycleId");
    // value travels as a JSON number, not a string.
    expect(typeof body.value).toBe("number");
  });

  it("throws on a non-ok response (e.g. server range 400)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 400 })));
    await expect(
      captureGrade(BASE, { assignmentId: "asg1", enrollmentId: "en1", termId: "t1", value: 11 }),
    ).rejects.toThrow();
  });
});

describe("getRoster", () => {
  it("GETs /grades/roster with credentials, the query params, and unwraps {roster}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ roster: [rosterRow] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await getRoster(BASE, { assignmentId: "asg1", termId: "t1" });

    expect(res).toEqual([rosterRow]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades/roster?assignmentId=asg1&termId=t1");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(getRoster(BASE, { assignmentId: "asg1", termId: "t1" })).resolves.toEqual([]);
  });

  it("throws on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 403 })));
    await expect(getRoster(BASE, { assignmentId: "asg1", termId: "t1" })).rejects.toThrow();
  });
});

describe("listGradeEntries", () => {
  it("GETs /grades with credentials, the query params, and unwraps {gradeEntries}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ gradeEntries: [gradeEntry] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listGradeEntries(BASE, { enrollmentId: "en1", termId: "t1" });

    expect(res).toEqual([gradeEntry]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades?enrollmentId=en1&termId=t1");
    expect(init.credentials).toBe("include");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listGradeEntries(BASE, { enrollmentId: "en1", termId: "t1" })).resolves.toEqual([]);
  });
});

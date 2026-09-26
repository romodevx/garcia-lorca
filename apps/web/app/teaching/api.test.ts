import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// NEXT_PUBLIC_* is inlined at BUILD time, so app/api-url.ts resolves the base URL
// once when the module is first imported — not on every call. The env therefore
// has to be stubbed BEFORE importing, which is also what production actually
// does. Stubbing afterwards used to appear to work only because the old helper
// re-read process.env on every request.
let captureGrade: typeof import("./api").captureGrade;
let getRoster: typeof import("./api").getRoster;
let listAssignments: typeof import("./api").listAssignments;
let listAttendanceClasses: typeof import("./api").listAttendanceClasses;
let getTeachingAttendanceSheet: typeof import("./api").getTeachingAttendanceSheet;
let saveTeachingAttendance: typeof import("./api").saveTeachingAttendance;
let listTerms: typeof import("./api").listTerms;

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
});

beforeEach(async () => {
  vi.stubEnv("NEXT_PUBLIC_API_URL", "http://gw");
  vi.resetModules();
  ({ captureGrade, getRoster, listAssignments, listTerms, listAttendanceClasses, getTeachingAttendanceSheet, saveTeachingAttendance } =
    await import("./api"));
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

// --- attendance (self-scoped) ---------------------------------------------

const dailyClass = { groupId: "g1", groupLabel: "1° SECUNDARIA · A", levelName: "Secundaria", mode: "daily" };
const perClass = {
  groupId: "g2",
  groupLabel: "2° SECUNDARIA · B",
  levelName: "Secundaria",
  mode: "per_class",
  subjectId: "s-mat",
  subjectName: "Matemáticas",
  assignmentId: "a1",
};

const sheet = {
  mode: "daily",
  date: "2026-09-01",
  editable: true,
  windowDays: 7,
  session: { takenAt: "2026-09-01T08:05:00", takenByName: "Mtra. Rojas" },
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "absent", note: "Cita" }],
};

describe("listAttendanceClasses", () => {
  it("GETs /teaching/attendance/classes with credentials and returns the array", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse([dailyClass, perClass]));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listAttendanceClasses()).resolves.toEqual([dailyClass, perClass]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching/attendance/classes");
    expect(init.credentials).toBe("include");
  });

  it("also accepts a {classes} envelope", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ classes: [perClass] })));
    await expect(listAttendanceClasses()).resolves.toEqual([perClass]);
  });

  it("throws the server's text on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("módulo apagado", { status: 404 })));
    await expect(listAttendanceClasses()).rejects.toThrow("módulo apagado");
  });
});

describe("getTeachingAttendanceSheet", () => {
  it("GETs the roster with groupId, date and (per class) subjectId, returning the sheet", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sheet));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getTeachingAttendanceSheet({ groupId: "g1", date: "2026-09-01", subjectId: "s-mat" })).resolves.toEqual(sheet);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching/attendance/roster?groupId=g1&date=2026-09-01&subjectId=s-mat");
    expect(init.credentials).toBe("include");
  });

  it("omits subjectId for a daily list", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sheet));
    vi.stubGlobal("fetch", fetchMock);

    await getTeachingAttendanceSheet({ groupId: "g1", date: "2026-09-01" });
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/teaching/attendance/roster?groupId=g1&date=2026-09-01");
  });
});

describe("saveTeachingAttendance", () => {
  it("POSTs groupId, date, subjectId and the exception marks with their notes", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await saveTeachingAttendance({
      groupId: "g2",
      date: "2026-09-01",
      subjectId: "s-mat",
      marks: [
        { enrollmentId: "en1", status: "excused", note: "Cita médica" },
        { enrollmentId: "en2", status: "absent", note: "" },
      ],
    });

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/teaching/attendance");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({
      groupId: "g2",
      date: "2026-09-01",
      subjectId: "s-mat",
      marks: [
        { enrollmentId: "en1", status: "excused", note: "Cita médica" },
        { enrollmentId: "en2", status: "absent" },
      ],
    });
  });

  it("leaves subjectId out of a daily list and accepts a 200", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}));
    vi.stubGlobal("fetch", fetchMock);

    await saveTeachingAttendance({ groupId: "g1", date: "2026-09-01", marks: [] });
    expect(JSON.parse(lastCall(fetchMock)[1].body as string)).toEqual({ groupId: "g1", date: "2026-09-01", marks: [] });
  });

  it("rejects with the server's text/plain reason and its status", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Solo puedes corregir hasta 7 días atrás\n", { status: 400 })));
    const err = await saveTeachingAttendance({ groupId: "g1", date: "2026-08-01", marks: [] }).catch((e: unknown) => e);
    // vi.resetModules() gives the client its own copy of the error class, so it is
    // matched by name rather than instanceof.
    expect(err).toMatchObject({ name: "AttendanceRequestError", message: "Solo puedes corregir hasta 7 días atrás", status: 400 });
  });
});

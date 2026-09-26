import { afterEach, describe, expect, it, vi } from "vitest";
import { getAttendanceSheet, listAttendance, saveAttendanceSheet } from "./api";

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

const record = { id: "at1", enrollmentId: "en1", date: "2026-08-06", status: "absent" as const, note: "cita médica" };

describe("listAttendance", () => {
  it("GETs /attendance with credentials, the query params, and unwraps {records}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ records: [record] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listAttendance(BASE, { enrollmentId: "en1", from: "2026-08-01", to: "2026-08-31" });

    expect(res).toEqual([record]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/attendance?enrollmentId=en1&from=2026-08-01&to=2026-08-31");
    expect(init.credentials).toBe("include");
  });

  it("drops the empty optional filters from the query string", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ records: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listAttendance(BASE, { enrollmentId: "en1" });

    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/attendance?enrollmentId=en1");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listAttendance(BASE, { enrollmentId: "en1" })).resolves.toEqual([]);
  });
});

// --- the attendance sheet (director, per-level mode) ------------------------

describe("getAttendanceSheet", () => {
  const sheet = { mode: "per_class", date: "2026-09-01", editable: true, windowDays: 7, session: null, rows: [] };

  it("GETs /attendance/roster with groupId, date and subjectId, returning the sheet as is", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sheet));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getAttendanceSheet(BASE, { groupId: "g1", date: "2026-09-01", subjectId: "s1" })).resolves.toEqual(sheet);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/attendance/roster?groupId=g1&date=2026-09-01&subjectId=s1");
    expect(init.credentials).toBe("include");
  });

  it("throws the server's text on failure", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Esta lista es por clase: elige la asignatura", { status: 400 })));
    await expect(getAttendanceSheet(BASE, { groupId: "g1", date: "2026-09-01" })).rejects.toThrow(
      "Esta lista es por clase: elige la asignatura",
    );
  });
});

describe("saveAttendanceSheet", () => {
  it("POSTs /attendance with the exception marks (notes only when typed) and accepts a 204", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await saveAttendanceSheet(BASE, {
      groupId: "g1",
      date: "2026-09-01",
      subjectId: "s1",
      marks: [{ enrollmentId: "en1", status: "late", note: "  " }],
    });

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/attendance");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({
      groupId: "g1",
      date: "2026-09-01",
      subjectId: "s1",
      marks: [{ enrollmentId: "en1", status: "late" }],
    });
  });

  it("falls back to the status when the body is empty", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 500 })));
    await expect(saveAttendanceSheet(BASE, { groupId: "g1", date: "2026-09-01", marks: [] })).rejects.toThrow("HTTP 500");
  });
});

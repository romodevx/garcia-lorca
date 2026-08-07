import { afterEach, describe, expect, it, vi } from "vitest";
import { getDailyRoster, listAttendance, saveDailyAttendance } from "./api";

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

const rosterRow = { enrollmentId: "en1", studentId: "st1", status: "present" as const };
const record = { id: "at1", enrollmentId: "en1", date: "2026-08-06", status: "absent" as const, note: "cita médica" };

describe("getDailyRoster", () => {
  it("GETs /attendance/roster with credentials, the query params, and unwraps {roster}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ roster: [rosterRow] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await getDailyRoster(BASE, { groupId: "grp1", date: "2026-08-06" });

    expect(res).toEqual([rosterRow]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/attendance/roster?groupId=grp1&date=2026-08-06");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(getDailyRoster(BASE, { groupId: "grp1", date: "2026-08-06" })).resolves.toEqual([]);
  });

  it("throws on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 403 })));
    await expect(getDailyRoster(BASE, { groupId: "grp1", date: "2026-08-06" })).rejects.toThrow();
  });
});

describe("saveDailyAttendance", () => {
  it("POSTs {groupId, date, marks} carrying ONLY the exceptions and returns the counts", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ absentCount: 1, markedCount: 2 }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await saveDailyAttendance(BASE, {
      groupId: "grp1",
      date: "2026-08-06",
      marks: [
        { enrollmentId: "en1", status: "absent" },
        { enrollmentId: "en2", status: "late", note: "llegó 8:20" },
      ],
    });

    expect(res).toEqual({ absentCount: 1, markedCount: 2 });
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/attendance");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({
      groupId: "grp1",
      date: "2026-08-06",
      marks: [
        { enrollmentId: "en1", status: "absent" },
        { enrollmentId: "en2", status: "late", note: "llegó 8:20" },
      ],
    });
    // DEFAULT-PRESENT: a present student is an omission, never a mark.
    expect(body.marks.some((m: { status: string }) => m.status === "present")).toBe(false);
  });

  it("sends an empty marks array when the whole group is present", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ absentCount: 0, markedCount: 0 }));
    vi.stubGlobal("fetch", fetchMock);

    await saveDailyAttendance(BASE, { groupId: "grp1", date: "2026-08-06", marks: [] });

    const body = JSON.parse(lastCall(fetchMock)[1].body as string);
    expect(body).toEqual({ groupId: "grp1", date: "2026-08-06", marks: [] });
  });

  it("throws on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 400 })));
    await expect(
      saveDailyAttendance(BASE, { groupId: "grp1", date: "2026-08-06", marks: [] }),
    ).rejects.toThrow();
  });
});

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

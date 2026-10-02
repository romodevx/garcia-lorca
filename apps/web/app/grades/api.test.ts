import { afterEach, describe, expect, it, vi } from "vitest";

import { gradeSheetClient } from "./api";
import { GradeRequestError } from "./requestError";

const BASE = "http://gw/";

afterEach(() => vi.restoreAllMocks());

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

/** Returns the [url, init] of the single fetch call the helper made. */
function lastCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

const sheet = {
  assignmentId: "a1",
  periodId: "p1",
  periods: [{ id: "p1", ordinal: 1, name: "1.er trimestre", startsOn: "2026-08-24", endsOn: "2026-11-30" }],
  currentPeriodId: "p1",
  today: "2026-10-01",
  closedPeriodIds: [],
  scale: { min: 5, max: 10, decimals: 0, passing: 6 },
  gradingEnabled: true,
  editable: true,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", value: 9, updatedAt: "2026-09-10T10:00:00Z", rowEditable: true }],
};

describe("gradeSheetClient (director)", () => {
  it("load GETs /grades/period-sheet with the assignment and period", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sheet));
    vi.stubGlobal("fetch", fetchMock);

    await expect(gradeSheetClient(BASE).load({ assignmentId: "a1", periodId: "p1" })).resolves.toEqual(sheet);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades/period-sheet?assignmentId=a1&periodId=p1");
    expect(init.credentials).toBe("include");
  });

  it("load leaves the period to the server when none is chosen", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sheet));
    vi.stubGlobal("fetch", fetchMock);

    await gradeSheetClient(BASE).load({ assignmentId: "a1" });
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/grades/period-sheet?assignmentId=a1");
  });

  it("save PUTs /grades/period-grades with {assignmentId, periodId, enrollmentId, value}", async () => {
    const saved = { enrollmentId: "en1", periodId: "p1", value: 8, updatedAt: "2026-09-11T10:00:00Z" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(saved));
    vi.stubGlobal("fetch", fetchMock);

    await expect(gradeSheetClient(BASE).save({ assignmentId: "a1", periodId: "p1", enrollmentId: "en1", value: 8 })).resolves.toEqual(saved);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades/period-grades");
    expect(init.method).toBe("PUT");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ assignmentId: "a1", periodId: "p1", enrollmentId: "en1", value: 8 });
  });

  it("remove DELETEs /grades/period-grades with the key in the query and accepts a 204", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(gradeSheetClient(BASE).remove({ assignmentId: "a1", periodId: "p1", enrollmentId: "en1" })).resolves.toBeUndefined();
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades/period-grades?assignmentId=a1&periodId=p1&enrollmentId=en1");
    expect(init.method).toBe("DELETE");
  });

  it("rejects with the server's text/plain reason, its status and its code", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Solo se permiten calificaciones enteras", { status: 400, headers: { "X-Error-Code": "GRADE_NOT_INTEGER" } })));

    const failure = gradeSheetClient(BASE).save({ assignmentId: "a1", periodId: "p1", enrollmentId: "en1", value: 9.5 });
    await expect(failure).rejects.toBeInstanceOf(GradeRequestError);
    await expect(failure).rejects.toMatchObject({ status: 400, message: "Solo se permiten calificaciones enteras", code: "GRADE_NOT_INTEGER" });
  });

  it("a closed period refuses with 409 PERIOD_LOCKED", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("El periodo está cerrado", { status: 409, headers: { "X-Error-Code": "PERIOD_LOCKED" } })));
    await expect(gradeSheetClient(BASE).remove({ assignmentId: "a1", periodId: "p1", enrollmentId: "en1" })).rejects.toMatchObject({ status: 409, code: "PERIOD_LOCKED" });
  });
});

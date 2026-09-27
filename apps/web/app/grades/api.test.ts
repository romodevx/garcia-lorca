import { afterEach, describe, expect, it, vi } from "vitest";

import { GradeRequestError } from "./monthly";
import { gradeSheetClient } from "./api";

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
  month: "2026-09",
  months: ["2026-08", "2026-09"],
  scale: { min: 5, max: 10, decimals: 1, passing: 6 },
  editable: true,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", value: 9.5, updatedAt: "2026-09-10T10:00:00Z" }],
};

describe("gradeSheetClient (director)", () => {
  it("load GETs /grades/sheet with the assignment and month", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sheet));
    vi.stubGlobal("fetch", fetchMock);

    await expect(gradeSheetClient(BASE).load({ assignmentId: "a1", month: "2026-09" })).resolves.toEqual(sheet);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades/sheet?assignmentId=a1&month=2026-09");
    expect(init.credentials).toBe("include");
  });

  it("load leaves the month to the server when none is chosen", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sheet));
    vi.stubGlobal("fetch", fetchMock);

    await gradeSheetClient(BASE).load({ assignmentId: "a1" });
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/grades/sheet?assignmentId=a1");
  });

  it("save PUTs /grades/monthly with {assignmentId, month, enrollmentId, value}", async () => {
    const saved = { enrollmentId: "en1", month: "2026-09", value: 8, updatedAt: "2026-09-11T10:00:00Z" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(saved));
    vi.stubGlobal("fetch", fetchMock);

    await expect(gradeSheetClient(BASE).save({ assignmentId: "a1", month: "2026-09", enrollmentId: "en1", value: 8 })).resolves.toEqual(saved);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades/monthly");
    expect(init.method).toBe("PUT");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ assignmentId: "a1", month: "2026-09", enrollmentId: "en1", value: 8 });
  });

  it("remove DELETEs /grades/monthly with the key in the query and accepts a 204", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(gradeSheetClient(BASE).remove({ assignmentId: "a1", month: "2026-09", enrollmentId: "en1" })).resolves.toBeUndefined();
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/grades/monthly?assignmentId=a1&month=2026-09&enrollmentId=en1");
    expect(init.method).toBe("DELETE");
  });

  it("rejects with the server's text/plain reason and its status", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("La calificación debe estar entre 5 y 10", { status: 400 })));

    const failure = gradeSheetClient(BASE).save({ assignmentId: "a1", month: "2026-09", enrollmentId: "en1", value: 11 });
    await expect(failure).rejects.toBeInstanceOf(GradeRequestError);
    await expect(failure).rejects.toMatchObject({ status: 400, message: "La calificación debe estar entre 5 y 10" });
  });

  it("load rejects with the server's reason too", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Este nivel no lleva calificación numérica", { status: 400 })));
    await expect(gradeSheetClient(BASE).load({ assignmentId: "a1" })).rejects.toThrow("Este nivel no lleva calificación numérica");
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";

import { scanAttendanceCard } from "./api";
import { readScanResponse } from "./scan";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

const text = (body: string, status: number, headers: Record<string, string> = {}): Response =>
  new Response(body, { status, headers: { "Content-Type": "text/plain", ...headers } });

describe("readScanResponse", () => {
  it("reads a 200 into the scanned student", async () => {
    const res = new Response(JSON.stringify({ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace" }), { status: 200 });
    await expect(readScanResponse(res)).resolves.toEqual({ kind: "ok", student: { enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace" } });
  });

  it.each([
    [422, "invalid", "Código no válido", "invalid"],
    [422, "foreign", "Este alumno no pertenece a este grupo", "foreign"],
    [409, undefined, "Credencial suspendida", "suspended"],
  ])("a %i (X-Scan-Result %s) is a rejection with the server's words", async (status, header, body, reason) => {
    const res = text(body, status, header !== undefined ? { "X-Scan-Result": header } : {});
    await expect(readScanResponse(res)).resolves.toEqual({ kind: "rejected", reason, message: body });
  });

  it("falls back to its own words when the server sent none", async () => {
    await expect(readScanResponse(text("", 422, { "X-Scan-Result": "invalid" }))).resolves.toMatchObject({ reason: "invalid", message: "Código no válido" });
    await expect(readScanResponse(text("", 409))).resolves.toMatchObject({ reason: "suspended", message: "Credencial suspendida" });
    await expect(readScanResponse(text("", 403))).resolves.toMatchObject({ reason: "denied" });
  });

  it("treats an unknown status or header defensively: the text body as an error", async () => {
    await expect(readScanResponse(text("Algo raro", 418))).resolves.toEqual({ kind: "rejected", reason: "unknown", message: "Algo raro" });
    await expect(readScanResponse(text("Otra cosa", 422, { "X-Scan-Result": "weird" }))).resolves.toEqual({ kind: "rejected", reason: "unknown", message: "Otra cosa" });
    await expect(readScanResponse(text("", 500))).resolves.toMatchObject({ reason: "unknown", message: "No se pudo leer la credencial. Intenta de nuevo." });
  });

  it("a 200 without the student's fields is not a success", async () => {
    await expect(readScanResponse(new Response("{}", { status: 200 }))).resolves.toMatchObject({ kind: "rejected", reason: "unknown" });
  });
});

describe("scanAttendanceCard", () => {
  it("POSTs the code to /attendance/scan with credentials, subjectId only when set", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ enrollmentId: "en1", studentId: "st1", name: "Ada" }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const out = await scanAttendanceCard(BASE, { groupId: "g1", code: "GLCARD:abc" });

    expect(out).toMatchObject({ kind: "ok" });
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/attendance/scan");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ groupId: "g1", code: "GLCARD:abc" });
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";

import { StudentsError, revokeCard, verifyCard } from "./api";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

describe("student card (tarea 13)", () => {
  it("verifies a scanned code, sent in the body and never the URL", async () => {
    const verified = { valid: true, studentId: "st1", name: "Zoe Villalobos", level: "Primaria", gradeGroup: "1° A", enrollmentNumber: "2026-00042", admissionStatus: "enrolled" };
    const mock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify(verified), { status: 200 }));

    expect(await verifyCard(BASE, "  GLCARD:AQAAAAAAAAAAAAAAAAAAAA\n")).toEqual(verified);
    const [u, init] = mock.mock.calls[0] as [string, RequestInit];
    expect(u).toBe("http://gw/api/v1/students/card/verify");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ code: "GLCARD:AQAAAAAAAAAAAAAAAAAAAA" });
  });

  it("says an unknown code is unknown", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("not found", { status: 404 }));
    await expect(verifyCard(BASE, "GLCARD:ZZZZZZZZZZZZZZZZZZZZZZ")).rejects.toMatchObject({ status: 404 });
  });

  it("revokes a pupil's card", async () => {
    const mock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));
    await revokeCard(BASE, "st1");
    const [u, init] = mock.mock.calls[0] as [string, RequestInit];
    expect(u).toBe("http://gw/api/v1/students/st1/card/revoke");
    expect(init.method).toBe("POST");
  });

  it("surfaces a refused revoke", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("forbidden", { status: 403 }));
    await expect(revokeCard(BASE, "st1")).rejects.toBeInstanceOf(StudentsError);
  });
});

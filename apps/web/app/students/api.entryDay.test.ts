import { afterEach, describe, expect, it, vi } from "vitest";
import { enroll, enrollIntoGrade } from "./api";

const BASE = "http://gw";

afterEach(() => vi.unstubAllGlobals());

function created(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 201, headers: { "Content-Type": "application/json" } });
}

function sentBody(mock: ReturnType<typeof vi.fn>): Record<string, unknown> {
  const init = (mock.mock.calls[0] as [string, RequestInit])[1];
  return JSON.parse(init.body as string) as Record<string, unknown>;
}

// T9: the entry day travels only when the office chose one. Omitted, the server
// takes today — which is why a user without finance:write is unaffected.
describe("the entry day on an enrolment", () => {
  it("by grade: sends enrolledOn when given, and nothing when not", async () => {
    const fetchMock = vi.fn().mockImplementation(async () => created({ enrollment: {}, groupName: "1° B" }));
    vi.stubGlobal("fetch", fetchMock);

    await enrollIntoGrade(BASE, "st1", "gl1", "2026-08-24");
    expect(sentBody(fetchMock)).toEqual({ studentId: "st1", gradeLevelId: "gl1", enrolledOn: "2026-08-24" });

    fetchMock.mockClear();
    await enrollIntoGrade(BASE, "st1", "gl1");
    expect(sentBody(fetchMock)).toEqual({ studentId: "st1", gradeLevelId: "gl1" });
  });

  it("into a group: sends enrolledOn when given, and nothing when not", async () => {
    const fetchMock = vi.fn().mockImplementation(async () => created({}));
    vi.stubGlobal("fetch", fetchMock);

    await enroll(BASE, { studentId: "st1", groupId: "grp1", enrolledOn: "2026-08-24" });
    expect(sentBody(fetchMock)).toEqual({ studentId: "st1", groupId: "grp1", enrolledOn: "2026-08-24" });

    fetchMock.mockClear();
    await enroll(BASE, { studentId: "st1", groupId: "grp1", enrolledOn: "" });
    expect(sentBody(fetchMock)).toEqual({ studentId: "st1", groupId: "grp1" });
  });
});

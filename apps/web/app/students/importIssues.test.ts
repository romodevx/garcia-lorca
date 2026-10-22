import { afterEach, describe, expect, it, vi } from "vitest";

import { StudentsError } from "./api";
import { acceptImportIssue, isDraft, issuesLabel, listImportIssues, type StudentImportIssue } from "./importIssues";

const ISSUE: StudentImportIssue = {
  id: "i1", studentId: "s1", field: "phone_mobile", code: "phone_invalid", originalValue: "55 12", storedValue: "",
  createdAt: "2026-10-21T10:00:00Z", resolution: "",
};

afterEach(() => vi.unstubAllGlobals());

function stubFetch(response: Response): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockResolvedValue(response);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("listImportIssues", () => {
  it("GETs the pupil's open import issues", async () => {
    const fetchMock = stubFetch(new Response(JSON.stringify({ issues: [ISSUE] }), { status: 200 }));
    expect(await listImportIssues("http://gw/", "s 1")).toEqual([ISSUE]);
    const [target, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(target).toBe("http://gw/api/v1/students/s%201/import-issues");
    expect(init.credentials).toBe("include");
  });

  it("throws a StudentsError on a refusal", async () => {
    stubFetch(new Response("not found", { status: 404 }));
    await expect(listImportIssues("http://gw", "s1")).rejects.toBeInstanceOf(StudentsError);
  });
});

describe("acceptImportIssue", () => {
  it("POSTs «está bien así» and returns the closed issue", async () => {
    const fetchMock = stubFetch(new Response(JSON.stringify({ ...ISSUE, resolution: "accepted" }), { status: 200 }));
    expect((await acceptImportIssue("http://gw", "s1", "i1")).resolution).toBe("accepted");
    const [target, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(target).toBe("http://gw/api/v1/students/s1/import-issues/i1/accept");
    expect(init.method).toBe("POST");
  });

  it("keeps the refusal's code (409 STUDENT_DRAFT)", async () => {
    stubFetch(new Response("draft", { status: 409, headers: { "X-Error-Code": "STUDENT_DRAFT" } }));
    await expect(acceptImportIssue("http://gw", "s1", "i1")).rejects.toMatchObject({ status: 409, code: "STUDENT_DRAFT" });
  });
});

describe("helpers", () => {
  it("knows a draft by its flag or its status", () => {
    expect(isDraft({ draft: true, admissionStatus: "pending" })).toBe(true);
    expect(isDraft({ admissionStatus: "draft" })).toBe(true);
    expect(isDraft({ draft: false, admissionStatus: "enrolled" })).toBe(false);
  });
  it("says the count in words", () => {
    expect(issuesLabel(1)).toBe("1 aviso de importación");
    expect(issuesLabel(3)).toBe("3 avisos de importación");
  });
});

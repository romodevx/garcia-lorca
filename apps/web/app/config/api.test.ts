import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createCycle,
  createGradeLevel,
  createGroup,
  createLevel,
  createSubject,
  createTerm,
  listCycles,
  listGradeLevels,
  listGroups,
  listLevels,
  listSubjects,
  listTerms,
} from "./api";

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

describe("createCycle", () => {
  it("POSTs the body with credentials and returns the created cycle", async () => {
    const created = { id: "c1", name: "2026-2027", status: "draft", startsOn: "2026-08-01", endsOn: "2027-06-30" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createCycle(BASE, { name: "2026-2027", startsOn: "2026-08-01", endsOn: "2027-06-30" });

    expect(res).toEqual(created);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/cycles");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ name: "2026-2027", startsOn: "2026-08-01", endsOn: "2027-06-30" });
  });

  it("throws on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 403 })));
    await expect(createCycle(BASE, { name: "x", startsOn: "2026-08-01", endsOn: "2027-06-30" })).rejects.toThrow();
  });
});

describe("listCycles", () => {
  it("GETs with credentials and unwraps the {cycles} envelope", async () => {
    const cycles = [{ id: "c1", name: "2026-2027", status: "draft", startsOn: "2026-08-01", endsOn: "2027-06-30" }];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ cycles }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listCycles(BASE);

    expect(res).toEqual(cycles);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/cycles");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listCycles(BASE)).resolves.toEqual([]);
  });
});

describe("levels", () => {
  it("createLevel POSTs name/sepCode/enrollmentGranularity", async () => {
    const created = { id: "l1", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createLevel(BASE, { name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" });

    expect(res).toEqual(created);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/levels");
    expect(JSON.parse(init.body as string)).toEqual({ name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" });
  });

  it("listLevels unwraps {levels}", async () => {
    const levels = [{ id: "l1", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" }];
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ levels })));
    await expect(listLevels(BASE)).resolves.toEqual(levels);
  });
});

describe("grade-levels", () => {
  it("createGradeLevel POSTs levelId/name/ordinal", async () => {
    const created = { id: "g1", levelId: "l1", name: "1° primaria", ordinal: 1 };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createGradeLevel(BASE, { levelId: "l1", name: "1° primaria", ordinal: 1 });

    expect(res).toEqual(created);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/grade-levels");
    expect(JSON.parse(init.body as string)).toEqual({ levelId: "l1", name: "1° primaria", ordinal: 1 });
  });

  it("listGradeLevels filters by levelId query param", async () => {
    const gradeLevels = [{ id: "g1", levelId: "l1", name: "1° primaria", ordinal: 1 }];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ gradeLevels }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listGradeLevels(BASE, "l1");

    expect(res).toEqual(gradeLevels);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/grade-levels?levelId=l1");
  });
});

describe("subjects", () => {
  it("createSubject POSTs gradeLevelId/name", async () => {
    const created = { id: "s1", gradeLevelId: "g1", name: "Matemáticas" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createSubject(BASE, { gradeLevelId: "g1", name: "Matemáticas" });

    expect(res).toEqual(created);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/subjects");
  });

  it("listSubjects filters by gradeLevelId query param", async () => {
    const subjects = [{ id: "s1", gradeLevelId: "g1", name: "Matemáticas" }];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ subjects }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listSubjects(BASE, "g1")).resolves.toEqual(subjects);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/subjects?gradeLevelId=g1");
  });
});

describe("terms", () => {
  it("createTerm POSTs the full term body", async () => {
    const created = { id: "t1", cycleId: "c1", levelId: "l1", name: "T1", ordinal: 1, startsOn: "2026-08-01", endsOn: "2026-12-15" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createTerm(BASE, { cycleId: "c1", levelId: "l1", name: "T1", ordinal: 1, startsOn: "2026-08-01", endsOn: "2026-12-15" });

    expect(res).toEqual(created);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/terms");
  });

  it("listTerms includes cycleId and optional levelId query params", async () => {
    const terms = [{ id: "t1", cycleId: "c1", levelId: "l1", name: "T1", ordinal: 1, startsOn: "2026-08-01", endsOn: "2026-12-15" }];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ terms }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listTerms(BASE, "c1", "l1")).resolves.toEqual(terms);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/terms?cycleId=c1&levelId=l1");
  });

  it("listTerms omits levelId when not given", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ terms: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listTerms(BASE, "c1");
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/terms?cycleId=c1");
  });
});

describe("groups", () => {
  it("createGroup POSTs cycleId/gradeLevelId/name/capacity", async () => {
    const created = { id: "grp1", cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createGroup(BASE, { cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 });

    expect(res).toEqual(created);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/groups");
    expect(JSON.parse(init.body as string)).toEqual({ cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 });
  });

  it("listGroups includes cycleId and optional gradeLevelId query params", async () => {
    const groups = [{ id: "grp1", cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 }];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ groups }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listGroups(BASE, "c1", "g1")).resolves.toEqual(groups);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/groups?cycleId=c1&gradeLevelId=g1");
  });

  it("listGroups omits gradeLevelId when not given", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ groups: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listGroups(BASE, "c1");
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/groups?cycleId=c1");
  });
});

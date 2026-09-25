import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ConfigError,
  createCycle,
  createGradeLevel,
  createGroup,
  createLevel,
  createSubject,
  createTerm,
  getSchoolProfile,
  listCycles,
  listGradeLevels,
  listGroups,
  listLabelledGroups,
  listLevels,
  listSubjects,
  listTerms,
  saveSchoolProfile,
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


describe("what a failure tells the user", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  /**
   * These exist because "No se pudo crear el ciclo. Intenta de nuevo." was the
   * ONLY thing the catalog said, whatever went wrong.
   *
   * The two ways creating a cycle actually fails are a name that already exists
   * and an end date before the start — and for the first one, "intenta de nuevo"
   * is advice that will never work. The status is the only signal the client
   * gets, so it has to survive the throw.
   */
  it("carries the status so a duplicate can be told from bad dates", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Conflict", { status: 409 })));

    const err = (await createCycle("http://gw", {
      name: "Ciclo 2026-2027",
      startsOn: "2026-08-31",
      endsOn: "2027-07-09",
    }).catch((e: unknown) => e)) as ConfigError;

    expect(err).toBeInstanceOf(ConfigError);
    expect(err.status).toBe(409);
    expect(err.isConflict).toBe(true);
  });

  it("tells a bad request apart from a conflict", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Bad Request", { status: 400 })));

    const err = (await createCycle("http://gw", {
      name: "Raro",
      startsOn: "2028-07-10",
      endsOn: "2027-08-30",
    }).catch((e: unknown) => e)) as ConfigError;

    expect(err.status).toBe(400);
    expect(err.isConflict).toBe(false);
    expect(err.isInvalid).toBe(true);
  });

  it("keeps the server's words when it sends any", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("cycle name already used", { status: 409 })));

    const err = (await createCycle("http://gw", {
      name: "x",
      startsOn: "2026-01-01",
      endsOn: "2026-12-31",
    }).catch((e: unknown) => e)) as ConfigError;

    expect(err.detail).toBe("cycle name already used");
  });
});

// Task 12: the school's details, for the receipts.
describe("school profile", () => {
  const profile = {
    name: "Colegio García Lorca", legalName: "", rfc: "EGL150101AB1", cct: "",
    address: "", phone: "5512345678", legend: "",
  };

  it("reads the details and whether the school has filled them", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ profile, found: true }));
    vi.stubGlobal("fetch", fetchMock);

    const got = await getSchoolProfile(BASE);

    expect(got).toEqual({ profile, found: true });
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/school-profile");
  });

  it("PUTs every field and returns the saved (normalised) details", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ profile }));
    vi.stubGlobal("fetch", fetchMock);

    const saved = await saveSchoolProfile(BASE, { ...profile, rfc: "egl150101ab1" });

    const [u, init] = lastCall(fetchMock);
    expect(u).toBe("http://gw/api/v1/config/school-profile");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(String(init.body))).toEqual({ ...profile, rfc: "egl150101ab1" });
    expect(saved.rfc).toBe("EGL150101AB1");
  });

  it("carries the server's words when the details are refused", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("the RFC does not have the SAT's shape", { status: 400 })));

    await expect(saveSchoolProfile(BASE, profile)).rejects.toMatchObject({ status: 400, isInvalid: true });
  });
});

// QA 2026-09-25, ISSUE-002: every grade has a group "A", so a picker that shows
// only the letter offers "A, A, A, B" and nobody can tell them apart.
describe("listLabelledGroups", () => {
  afterEach(() => vi.unstubAllGlobals());
  const groups = [
    { id: "g1", cycleId: "c1", gradeLevelId: "k1", name: "A", capacity: 30 },
    { id: "g2", cycleId: "c1", gradeLevelId: "p1", name: "A", capacity: 2 },
    { id: "g3", cycleId: "c1", gradeLevelId: "p1", name: "B", capacity: 30 },
  ];
  const grades = [
    { id: "p1", levelId: "lp", name: "1° PRIMARIA", ordinal: 1 },
    { id: "k1", levelId: "lk", name: "1° KÍNDER", ordinal: 1 },
  ];

  it("names each group with its grade", async () => {
    vi.stubGlobal("fetch", vi.fn((url: string) =>
      Promise.resolve(jsonResponse(url.includes("/grade-levels") ? { gradeLevels: grades } : { groups }))));
    const out = await listLabelledGroups(BASE, "c1");
    expect(out.map((g) => g.label)).toEqual(["1° KÍNDER · A", "1° PRIMARIA · A", "1° PRIMARIA · B"]);
    expect(out[1]).toMatchObject({ id: "g2", name: "A", capacity: 2 });
  });

  it("falls back to the bare name when the grades cannot be read", async () => {
    vi.stubGlobal("fetch", vi.fn((url: string) =>
      Promise.resolve(url.includes("/grade-levels") ? jsonResponse({}, 500) : jsonResponse({ groups }))));
    const out = await listLabelledGroups(BASE, "c1");
    expect(out.map((g) => g.label)).toEqual(["A", "A", "B"]);
  });
});

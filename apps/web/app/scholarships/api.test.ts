import { afterEach, describe, expect, it, vi } from "vitest";

import {
  assignScholarship,
  createScholarship,
  getScholarship,
  listScholarshipHolders,
  listScholarships,
  listScholarshipTargets,
  unassignScholarship,
  updateScholarship,
  type ScholarshipInput,
} from "./api";

const BASE = "http://gw/";
const BECA = "b1000000-0000-0000-0000-000000000001";
const OTHER = "b1000000-0000-0000-0000-000000000002";
const STU = "51000000-0000-0000-0000-000000000001";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stub(body: unknown, status = 200): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation(() =>
    Promise.resolve(new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function stubText(text: string, status: number): void {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(text, { status, headers: { "Content-Type": "text/plain" } })));
}

function call(fetchMock: ReturnType<typeof vi.fn>, i = 0): [string, RequestInit] {
  return fetchMock.mock.calls[i] as [string, RequestInit];
}

const input: ScholarshipInput = { name: "BECA HERMANOS", kind: "percent", value: 5000, targets: ["tuition", "enrollment"], active: true };
const wire = { ...input, id: BECA, assignedCount: 14 };

describe("scholarships api (T18)", () => {
  it("lists the scholarships, inactive ones when asked", async () => {
    const fetchMock = stub({ scholarships: [wire] });
    const got = await listScholarships(BASE, true);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/scholarships?includeInactive=true");
    expect(got).toEqual([wire]);
  });

  it("a row missing its optional fields still reads whole", async () => {
    stub({ scholarships: [{ id: BECA, name: "X", kind: "fixed_off", value: 100, active: true }] });
    const [got] = await listScholarships(BASE);
    expect(got?.targets).toEqual([]);
    expect(got?.assignedCount).toBe(0);
  });

  it("an unknown kind degrades to percent instead of breaking the row", async () => {
    stub({ scholarships: [{ ...wire, kind: "voucher" }] });
    const [got] = await listScholarships(BASE);
    expect(got?.kind).toBe("percent");
  });

  it("creates with POST and edits with PUT, sending the contract's body", async () => {
    const fetchMock = stub(wire);
    await createScholarship(BASE, input);
    await updateScholarship(BASE, BECA, input);
    const [createURL, createInit] = call(fetchMock, 0);
    expect(createURL).toBe("http://gw/api/v1/scholarships");
    expect(createInit.method).toBe("POST");
    expect(JSON.parse(createInit.body as string)).toEqual(input);
    const [updateURL, updateInit] = call(fetchMock, 1);
    expect(updateURL).toBe(`http://gw/api/v1/scholarships/${BECA}`);
    expect(updateInit.method).toBe("PUT");
  });

  it("reads one scholarship", async () => {
    const fetchMock = stub(wire);
    expect((await getScholarship(BASE, BECA)).assignedCount).toBe(14);
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/scholarships/${BECA}`);
  });

  it("reads what a scholarship can apply to", async () => {
    const targets = [{ key: "tuition", label: "Colegiatura", group: "tuition" }];
    const fetchMock = stub({ targets });
    expect(await listScholarshipTargets(BASE)).toEqual(targets);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/scholarships/targets");
  });

  it("reads who holds it, with what else they hold", async () => {
    const holder = { studentId: STU, startsOn: "2026-10-02", overlaps: [{ scholarshipId: OTHER, name: "BECA EXCELENCIA" }] };
    const fetchMock = stub({ students: [holder, { studentId: OTHER, startsOn: "2026-10-02" }] });
    const got = await listScholarshipHolders(BASE, BECA);
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/scholarships/${BECA}/students`);
    expect(got[0]).toEqual(holder);
    expect(got[1]?.overlaps).toEqual([]);
  });

  it("gives it to pupils and takes it away", async () => {
    const fetchMock = stub({ students: [{ studentId: STU, startsOn: "2026-10-02", overlaps: [] }] });
    const holders = await assignScholarship(BASE, BECA, [STU]);
    expect(holders).toHaveLength(1);
    const [assignURL, assignInit] = call(fetchMock, 0);
    expect(assignURL).toBe(`http://gw/api/v1/scholarships/${BECA}/students`);
    expect(assignInit.method).toBe("POST");
    expect(JSON.parse(assignInit.body as string)).toEqual({ studentIds: [STU] });

    const removeMock = stub({ removed: 1 });
    expect(await unassignScholarship(BASE, BECA, [STU])).toBe(1);
    const [removeURL, removeInit] = call(removeMock, 0);
    expect(removeURL).toBe(`http://gw/api/v1/scholarships/${BECA}/students/remove`);
    expect(JSON.parse(removeInit.body as string)).toEqual({ studentIds: [STU] });
  });

  it("an id that is not one never reaches the network", async () => {
    const fetchMock = stub({});
    await expect(getScholarship(BASE, "../flags")).rejects.toThrow("Ese registro no existe.");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("says the gateway's sentence when it refuses", async () => {
    stubText("Ya existe una beca con ese nombre.", 409);
    await expect(createScholarship(BASE, input)).rejects.toThrow("Ya existe una beca con ese nombre.");
  });
});

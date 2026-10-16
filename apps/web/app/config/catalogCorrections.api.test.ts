import { afterEach, describe, expect, it, vi } from "vitest";

import { deleteGradeLevel, deleteGroup, renameGradeLevel, renameGroup, reorderLevels } from "./api";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function refusal(status: number, text: string, code: string): Response {
  return new Response(text, { status, headers: { "X-Error-Code": code } });
}

function onlyCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

describe("reorderLevels", () => {
  it("PUTs the whole ordered id list and returns the levels in their new order", async () => {
    const levels = [
      { id: "l2", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle", sortOrder: 1 },
      { id: "l1", name: "KINDER", sepCode: "", enrollmentGranularity: "cycle", sortOrder: 2 },
    ];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ levels }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(reorderLevels(BASE, ["l2", "l1"])).resolves.toEqual(levels);
    const [url, init] = onlyCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/levels/order");
    expect(init.method).toBe("PUT");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ levelIds: ["l2", "l1"] });
  });

  it("surfaces LEVEL_ORDER_MISMATCH with the server's sentence", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(refusal(400, "La lista no coincide.", "LEVEL_ORDER_MISMATCH")));
    await expect(reorderLevels(BASE, ["l1"])).rejects.toMatchObject({ code: "LEVEL_ORDER_MISMATCH", detail: "La lista no coincide." });
  });
});

describe("grade corrections", () => {
  it("renameGradeLevel PATCHes the name and returns the grade", async () => {
    const renamed = { id: "g1", levelId: "l1", name: "3°", ordinal: 3 };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(renamed));
    vi.stubGlobal("fetch", fetchMock);

    await expect(renameGradeLevel(BASE, "g1", "3°")).resolves.toEqual(renamed);
    const [url, init] = onlyCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/grade-levels/g1");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(init.body as string)).toEqual({ name: "3°" });
  });

  it("deleteGradeLevel DELETEs, and a 409 carries the blockers sentence", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(deleteGradeLevel(BASE, "g1")).resolves.toBeUndefined();
    expect(onlyCall(fetchMock)[0]).toBe("http://gw/api/v1/config/grade-levels/g1");
    expect(onlyCall(fetchMock)[1].method).toBe("DELETE");

    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(refusal(409, "No se puede borrar el grado 3°: tiene 2 grupos.", "GRADE_LEVEL_IN_USE")));
    await expect(deleteGradeLevel(BASE, "g1")).rejects.toMatchObject({
      isConflict: true,
      code: "GRADE_LEVEL_IN_USE",
      detail: "No se puede borrar el grado 3°: tiene 2 grupos.",
    });
  });
});

describe("group corrections", () => {
  it("renameGroup PATCHes the name and returns the group", async () => {
    const renamed = { id: "gr1", cycleId: "c1", gradeLevelId: "g1", name: "B", capacity: 30 };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(renamed));
    vi.stubGlobal("fetch", fetchMock);

    await expect(renameGroup(BASE, "gr1", "B")).resolves.toEqual(renamed);
    const [url, init] = onlyCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/groups/gr1");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(init.body as string)).toEqual({ name: "B" });
  });

  it("deleteGroup DELETEs, and a 409 carries the blockers sentence", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(deleteGroup(BASE, "gr1")).resolves.toBeUndefined();
    expect(onlyCall(fetchMock)[0]).toBe("http://gw/api/v1/config/groups/gr1");
    expect(onlyCall(fetchMock)[1].method).toBe("DELETE");

    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(refusal(409, "No se puede borrar el grupo A: tiene 25 inscripciones.", "GROUP_IN_USE")));
    await expect(deleteGroup(BASE, "gr1")).rejects.toMatchObject({ code: "GROUP_IN_USE", detail: "No se puede borrar el grupo A: tiene 25 inscripciones." });
  });
});

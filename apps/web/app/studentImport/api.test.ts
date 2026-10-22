import { afterEach, describe, expect, it, vi } from "vitest";

import { StudentImportError, commitStudentImport, previewStudentImport, type StudentImportResult } from "./api";

const BASE = "http://gw/";

const RESULT: StudentImportResult = {
  cycle: { id: "c1", name: "2026–2027" },
  levels: [],
  placements: [],
  rows: [],
  summary: { total: 0, new: 0, warnings: 0, already: 0, skipped: 0, pending: 0, movedToCycleStart: 0 },
  unresolved: [],
  planHash: "h1",
};

afterEach(() => vi.unstubAllGlobals());

function stubFetch(response: Response): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockResolvedValue(response);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("previewStudentImport", () => {
  it("POSTs the file, the level choices and the capacities with the session cookie", async () => {
    const fetchMock = stubFetch(new Response(JSON.stringify(RESULT), { status: 200 }));

    const out = await previewStudentImport(BASE, {
      fileName: "Primaria.xlsx",
      fileBase64: "UEsD",
      levels: { Preparatoria: { levelId: "l9" } },
      capacities: { "Primaria|01|A": 32 },
    });

    expect(out).toEqual(RESULT);
    const [target, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(target).toBe("http://gw/api/v1/imports/students/preview");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({
      fileName: "Primaria.xlsx",
      fileBase64: "UEsD",
      levels: { Preparatoria: { levelId: "l9" } },
      capacities: { "Primaria|01|A": 32 },
    });
  });

  it("turns a coded refusal into a StudentImportError with status, code and the body's words", async () => {
    stubFetch(new Response("CURP\n", { status: 400, headers: { "X-Error-Code": "FILE_MISSING_COLUMN" } }));

    const err = await previewStudentImport(BASE, { fileName: "x.xlsx", fileBase64: "UEsD" }).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(StudentImportError);
    expect(err).toMatchObject({ status: 400, code: "FILE_MISSING_COLUMN", detail: "CURP" });
  });

  it("leaves the code undefined when the gateway sent none", async () => {
    stubFetch(new Response("forbidden", { status: 403 }));

    const err = await previewStudentImport(BASE, { fileName: "x.xlsx", fileBase64: "UEsD" }).catch((e: unknown) => e);

    expect(err).toMatchObject({ status: 403, code: undefined });
  });
});

describe("commitStudentImport", () => {
  it("POSTs to /commit with the preview's planHash", async () => {
    const fetchMock = stubFetch(new Response(JSON.stringify({ ...RESULT, batchId: "b1" }), { status: 200 }));

    const out = await commitStudentImport(BASE, { fileName: "Primaria.xlsx", fileBase64: "UEsD", planHash: "h1" });

    expect(out.batchId).toBe("b1");
    const [target, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(target).toBe("http://gw/api/v1/imports/students/commit");
    expect(JSON.parse(init.body as string)).toMatchObject({ planHash: "h1" });
  });
});

describe("the import review (module_import_review)", () => {
  it("sends the row decisions when there are any", async () => {
    const fetchMock = stubFetch(new Response(JSON.stringify(RESULT), { status: 200 }));

    await previewStudentImport(BASE, { fileName: "x.xlsx", fileBase64: "UEsD", rows: [{ row: 14, action: "draft" }, { row: 20, overrides: { mobilePhone: "5512345678" } }] });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(JSON.parse(init.body as string)).toMatchObject({ rows: [{ row: 14, action: "draft" }, { row: 20, overrides: { mobilePhone: "5512345678" } }] });
  });

  it("reads each refused decision of a 422 IMPORT_ROWS_INVALID", async () => {
    const body = { error: { code: "invalid", message: "Revisa las correcciones de la importación", details: { rows: [{ row: 20, field: "mobilePhone", code: "phone_invalid" }] } } };
    stubFetch(new Response(JSON.stringify(body), { status: 422, headers: { "X-Error-Code": "IMPORT_ROWS_INVALID" } }));

    const err = await commitStudentImport(BASE, { fileName: "x.xlsx", fileBase64: "UEsD", planHash: "h1" }).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(StudentImportError);
    expect(err).toMatchObject({ status: 422, code: "IMPORT_ROWS_INVALID", rows: [{ row: 20, field: "mobilePhone", code: "phone_invalid" }] });
  });

  it("keeps no rows for any other refusal", async () => {
    stubFetch(new Response("choose where these levels go: Preparatoria", { status: 422, headers: { "X-Error-Code": "LEVELS_UNRESOLVED" } }));

    const err = await commitStudentImport(BASE, { fileName: "x.xlsx", fileBase64: "UEsD", planHash: "h1" }).catch((e: unknown) => e);

    expect(err).toMatchObject({ code: "LEVELS_UNRESOLVED", rows: [] });
  });
});

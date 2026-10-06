import { describe, expect, it } from "vitest";

import { NOT_XLSX, ONE_FILE, TOO_LARGE } from "./copy";
import { MAX_FILE_BYTES, canImportStudents, pickFile, readAsBase64 } from "./file";

const xlsx = (name = "Primaria.xlsx", bytes = 4): File => new File([new Uint8Array(bytes)], name);

describe("pickFile", () => {
  it("takes the one .xlsx it is given", () => {
    const f = xlsx();
    expect(pickFile([f])).toEqual({ file: f });
  });

  it("takes only the first of several and says one at a time", () => {
    const first = xlsx("Maternal.xlsx");
    expect(pickFile([first, xlsx("Primaria.xlsx")])).toEqual({ file: first, notice: ONE_FILE });
  });

  it("refuses what is not an .xlsx before sending anything, in any case of extension", () => {
    expect(pickFile([xlsx("Primaria.xls")])).toEqual({ error: NOT_XLSX });
    expect(pickFile([xlsx("PRIMARIA.XLSX")]).file).toBeDefined();
  });

  it("refuses a file over 2 MB", () => {
    expect(pickFile([xlsx("Primaria.xlsx", MAX_FILE_BYTES + 1)])).toEqual({ error: TOO_LARGE });
  });

  it("says nothing when nothing was dropped", () => {
    expect(pickFile([])).toEqual({});
  });
});

describe("readAsBase64", () => {
  it("returns the bytes as base64 without the data-URL prefix", async () => {
    const file = new File([new Uint8Array([80, 75, 3, 4])], "x.xlsx");
    await expect(readAsBase64(file)).resolves.toBe("UEsDBA==");
  });
});

describe("canImportStudents", () => {
  it("needs students:import; unknown permissions offer nothing", () => {
    expect(canImportStudents(["students:import"])).toBe(true);
    expect(canImportStudents(["students:read"])).toBe(false);
    expect(canImportStudents(undefined)).toBe(false);
  });
});

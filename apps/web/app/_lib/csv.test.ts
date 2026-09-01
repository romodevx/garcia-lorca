import { describe, expect, it, vi } from "vitest";
import { downloadCsv, toCsv } from "./csv";

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

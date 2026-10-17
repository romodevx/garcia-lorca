/**
 * "Exportar esta vista": whatever is on screen, as a file the school can open
 * in Excel.
 *
 * It is deliberately built from the rows already loaded rather than from a new
 * server call. What a person means by "export this" is the list they are
 * looking at — the filter they typed, the tab they picked — and a fresh query
 * would quietly return something else.
 */

import { schoolISODate } from "./schoolDay";

/** One column: the header a person reads and how to get its value from a row. */
export interface CsvColumn<T> {
  label: string;
  value: (row: T) => string;
}

/**
 * Renders rows as CSV.
 *
 * Two escaping rules matter, and both come from real damage. A value holding a
 * comma, a quote or a newline is quoted, or a pupil called "Cruz, Valentina"
 * becomes two columns and shifts every row after it. And a value with a leading
 * zero is written as an Excel text formula, because Excel reads a bare 0155…
 * as the number 155 — phone numbers, CURPs and folios all lose digits, silently,
 * and the person who opens the file has no way of knowing.
 */
export function toCsv<T>(columns: readonly CsvColumn<T>[], rows: readonly T[]): string {
  const header = columns.map((c) => escapeCell(c.label)).join(",");
  const body = rows.map((row) => columns.map((c) => escapeCell(c.value(row))).join(","));
  return [header, ...body].join("\r\n");
}

/**
 * The UTF-8 byte order mark, written as an escape: an invisible literal is one
 * editor or formatter away from vanishing, and nothing would notice.
 */
const UTF8_BOM = "﻿";

/**
 * The file's bytes. The BOM is what makes Excel read the file as UTF-8: without
 * it "Sánchez" opens as "SÃ¡nchez" on a Windows machine, which is most of them.
 */
export function csvBlob(csv: string): Blob {
  return new Blob([UTF8_BOM, csv], { type: "text/csv;charset=utf-8" });
}

/** Hands the file to the browser under a name a person can find again. */
export function downloadCsv(filename: string, csv: string): void {
  const blob = csvBlob(csv);
  const href = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = href;
  anchor.download = filename;
  anchor.style.display = "none";
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(href);
}

// A cell starting with one of these is a formula to Excel (OWASP CSV
// injection). Plain signed numbers ("-150.00") are left alone: they are money.
const FORMULA_START = /^[=+\-@\t\r]/;
const SIGNED_NUMBER = /^[+-]?\d+(\.\d+)?$/;

function escapeCell(raw: string): string {
  // The apostrophe is Excel's own "this is text" mark: shown nowhere, run never.
  const value = FORMULA_START.test(raw) && !SIGNED_NUMBER.test(raw) ? `'${raw}` : raw;
  if (/^0\d+$/.test(value)) {
    // Excel's own escape hatch for "this is text, not a number".
    return `="${value}"`;
  }
  if (/[",\n\r]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

/**
 * A filename a person can find again: what it is, and the school's day it was
 * taken (Morelia's — UTC's day already turns at 18:00 local).
 */
export function csvName(what: string, day: Date): string {
  return `${what}-${schoolISODate(day)}.csv`;
}

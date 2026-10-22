import { describe, expect, it } from "vitest";

import type { ImportRow } from "./api";
import {
  changedOverrides,
  correctionError,
  decisionRows,
  editableFields,
  inReviewTab,
  needsReview,
  reviewCounts,
  withAction,
  withOverrides,
} from "./review";

const row = (over: Partial<ImportRow>): ImportRow => ({ row: 2, name: "RUIZ, BETO", curp: "RUXB100101HDFZXT02", destination: "Primaria · 1° · A", status: "warning", ...over });

describe("decisions", () => {
  it("sends only what differs from a plain import, in row order", () => {
    let d = withAction({}, 9, "omit");
    d = withOverrides(d, 3, { mobilePhone: "5512345678" });
    d = withAction(d, 5, "draft");
    d = withAction(d, 5, undefined);
    expect(decisionRows(d)).toEqual([{ row: 3, overrides: { mobilePhone: "5512345678" } }, { row: 9, action: "omit" }]);
  });

  it("keeps a row's corrections when its action changes, and merges new ones", () => {
    let d = withOverrides({}, 3, { mobilePhone: "5512345678" });
    d = withAction(d, 3, "draft");
    d = withOverrides(d, 3, { email: "ana@colegio.mx" });
    expect(decisionRows(d)).toEqual([{ row: 3, action: "draft", overrides: { mobilePhone: "5512345678", email: "ana@colegio.mx" } }]);
  });
});

describe("editableFields", () => {
  it("offers one field per override the row's issues point at, prefilled with the file's value", () => {
    const r = row({
      issues: [
        { field: "phone_mobile", code: "phone_invalid", original: "55 12", stored: "" },
        { field: "surnames", code: "surnames_ambiguous", original: "DE LA CRUZ ANA", stored: "" },
        { field: "first_name", code: "first_name_curp_mismatch", original: "DE LA CRUZ ANA", stored: "" },
        { field: "birth_date", code: "birth_date_from_curp", original: "31/02/2010", stored: "2010-01-01" },
      ],
    });
    expect(editableFields(r).map((f) => [f.name, f.initial])).toEqual([
      ["mobilePhone", "55 12"],
      ["fullName", "DE LA CRUZ ANA"],
      ["birthDate", "2010-01-01"],
    ]);
  });

  it("prefills with the correction already made", () => {
    const r = row({ issues: [{ field: "email", code: "email_invalid", original: "ana@", stored: "" }] });
    expect(editableFields(r, { row: 2, overrides: { email: "ana@colegio.mx" } })[0]?.initial).toBe("ana@colegio.mx");
  });

  it("falls back to the row's name for a name issue without the file's text", () => {
    const r = row({ issues: [{ field: "surnames", code: "surnames_ambiguous", original: "", stored: "" }] });
    expect(editableFields(r)[0]?.initial).toBe("RUIZ, BETO");
  });

  it("offers the CURP for a row that can be a draft", () => {
    const r = row({ status: "skipped", reason: "curp_invalid", canDraft: true, curp: "", issues: [{ field: "curp", code: "curp_missing", original: "", stored: "" }] });
    expect(editableFields(r).map((f) => f.name)).toEqual(["curp"]);
  });
});

describe("changedOverrides", () => {
  it("sends only the values the office changed, trimmed", () => {
    const fields = editableFields(row({ issues: [
      { field: "phone_mobile", code: "phone_invalid", original: "55 12", stored: "" },
      { field: "email", code: "email_invalid", original: "ana@", stored: "" },
    ] }));
    expect(changedOverrides(fields, { mobilePhone: " 55 1234 5678 ", email: "ana@" })).toEqual({ mobilePhone: "55 1234 5678" });
  });
});

describe("correctionError", () => {
  const today = "2026-10-21";
  it.each([
    ["curp", "ABC", "La CURP lleva 18 caracteres con la forma oficial."],
    ["curp", "RUXB100101HDFZXT02", undefined],
    ["mobilePhone", "55 12", "Un teléfono lleva 10 dígitos."],
    ["homePhone", "+52 55 1234 5678", undefined],
    ["postalCode", "5800", "El código postal lleva 5 dígitos."],
    ["bankReference", "12A", "Sólo dígitos, hasta 20."],
    ["birthDate", "2030-01-01", "La fecha de nacimiento no puede ser futura."],
    ["fullName", "RUIZ BETO", "Escríbelo como APELLIDOS, NOMBRES."],
    ["street", "x".repeat(121), "Máximo 120 caracteres."],
    ["email", "ana@", "Revisa el correo: le falta algo después de la @."],
  ] as const)("%s «%s» → %s", (name, value, want) => {
    const got = correctionError(name, value, today);
    if (want === undefined) expect(got).toBeUndefined();
    else if (name === "email") expect(got).toBeDefined();
    else expect(got).toBe(want);
  });

  it("says nothing about an empty value: that is not a format error", () => {
    expect(correctionError("mobilePhone", "", today)).toBeUndefined();
  });
});

describe("reviewCounts and the review tabs", () => {
  const rows = [
    row({ row: 2, draft: true, status: "new" }),
    row({ row: 3, status: "skipped", reason: "omitted" }),
    row({ row: 4, status: "warning", issues: [{ field: "email", code: "email_invalid", original: "x", stored: "" }] }),
  ];
  it("counts drafts and omitted rows", () => {
    expect(reviewCounts(rows)).toEqual({ drafts: 1, omitted: 1 });
  });
  it("files a row under Borradores or Omitidas", () => {
    expect(rows.filter((r) => inReviewTab(r, "draft")).map((r) => r.row)).toEqual([2]);
    expect(rows.filter((r) => inReviewTab(r, "omitted")).map((r) => r.row)).toEqual([3]);
  });
});

describe("needsReview", () => {
  it("is a row with issues, one that may be a draft, or one already decided", () => {
    expect(needsReview(row({}))).toBe(false);
    expect(needsReview(row({ issues: [{ field: "email", code: "email_invalid", original: "x", stored: "" }] }))).toBe(true);
    expect(needsReview(row({ canDraft: true }))).toBe(true);
    expect(needsReview(row({ status: "skipped", reason: "omitted" }), { row: 2, action: "omit" })).toBe(true);
  });
});

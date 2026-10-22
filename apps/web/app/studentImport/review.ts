/**
 * The import review (module_import_review, PLAN-import-avisos D2): before
 * «Importar» the office may correct a row's flagged values, omit the row, or
 * keep a row without a valid CURP as a draft. Pure on purpose — the decisions,
 * the inline editor's fields and their format rules are tested here and the
 * components stay thin. The server re-validates every correction.
 */
import { emailShapeError } from "../_design/form/emailShape";
import { isCurpShape } from "../students/curpCheck";
import { MEXICAN_STATES, bankReferenceError, birthDateError, postalCodeError } from "../students/profileRules";
import type { ImportRow, IssueField, OverrideName, RowAction, RowDecision, RowOverrides } from "./api";
import { OVERRIDE_LABEL } from "./copy";

/** The decisions by spreadsheet row. */
export type Decisions = Readonly<Record<number, RowDecision>>;

/** Which correction settles each issue field (the gateway's Overrides.Fields). */
const OVERRIDE_OF: Readonly<Record<IssueField, OverrideName>> = {
  curp: "curp",
  birth_date: "birthDate",
  phone_home: "homePhone",
  phone_mobile: "mobilePhone",
  email: "email",
  postal_code: "postalCode",
  state: "state",
  bank_reference: "bankReference",
  surnames: "fullName",
  first_name: "fullName",
  address: "street",
};

/** The row's decision with its action replaced (undefined = import as it is). */
export function withAction(decisions: Decisions, row: number, action: RowAction | undefined): Decisions {
  const { action: _dropped, ...rest } = decisions[row] ?? { row };
  return { ...decisions, [row]: action === undefined ? { ...rest, row } : { ...rest, row, action } };
}

/** The row's decision with these corrections merged into the earlier ones. */
export function withOverrides(decisions: Decisions, row: number, overrides: RowOverrides): Decisions {
  const current = decisions[row] ?? { row };
  return { ...decisions, [row]: { ...current, overrides: { ...current.overrides, ...overrides } } };
}

function isPlainImport(d: RowDecision): boolean {
  const noAction = d.action === undefined || d.action === "import";
  return noAction && (d.overrides === undefined || Object.keys(d.overrides).length === 0);
}

/** What the request carries: only rows that differ from a plain import, in row order. */
export function decisionRows(decisions: Decisions): RowDecision[] {
  return Object.values(decisions)
    .filter((d) => !isPlainImport(d))
    .sort((a, b) => a.row - b.row);
}

export type FieldKind = "text" | "date" | "state";

export interface EditableField {
  name: OverrideName;
  label: string;
  kind: FieldKind;
  /** The correction already made, else the file's value. */
  initial: string;
}

const ISO_DAY = /^\d{4}-\d{2}-\d{2}$/;

function kindOf(name: OverrideName): FieldKind {
  if (name === "birthDate") return "date";
  if (name === "state") return "state";
  return "text";
}

/**
 * One field per correction the row's issues point at, in the issues' order,
 * prefilled. A date field can only hold a real day, so it starts from the one
 * the import would store (the CURP's), not the file's unreadable text.
 */
export function editableFields(row: ImportRow, decision?: RowDecision): EditableField[] {
  const out: EditableField[] = [];
  for (const issue of row.issues ?? []) {
    const name = OVERRIDE_OF[issue.field];
    if (name === undefined || out.some((f) => f.name === name)) continue;
    const corrected = decision?.overrides?.[name];
    let initial = corrected ?? issue.original;
    if (corrected === undefined && name === "birthDate") initial = ISO_DAY.test(issue.stored) ? issue.stored : "";
    if (corrected === undefined && name === "fullName" && initial === "") initial = row.name;
    // A state the list does not have would show as a blank choice and send the
    // file's unknown name back: start blank, so the office has to pick one.
    if (name === "state" && !(MEXICAN_STATES as readonly string[]).includes(initial)) initial = "";
    out.push({ name, label: OVERRIDE_LABEL[name], kind: kindOf(name), initial });
  }
  return out;
}

/** The corrections to send: only the values the office changed, trimmed. */
export function changedOverrides(fields: readonly EditableField[], values: Readonly<Partial<Record<OverrideName, string>>>): RowOverrides {
  const out: RowOverrides = {};
  for (const f of fields) {
    const value = (values[f.name] ?? f.initial).trim();
    if (value !== f.initial.trim()) out[f.name] = value;
  }
  return out;
}

const PHONE_SEPARATORS = /[\s\-.()+]/g;
const NATIONAL_PHONE = 10;
/** What a previous system left before the ten digits (students' profile_lenient.go). */
const PHONE_PREFIXES = ["521", "044", "045", "52"];
const MAX_ADDRESS = 120;

function phoneShapeError(raw: string): string | undefined {
  const digits = raw.replace(PHONE_SEPARATORS, "");
  const ok = /^\d+$/.test(digits) && (digits.length === NATIONAL_PHONE || PHONE_PREFIXES.some((p) => digits.length === p.length + NATIONAL_PHONE && digits.startsWith(p)));
  return ok ? undefined : "Un teléfono lleva 10 dígitos.";
}

/**
 * The format rule of one correction, checked on blur once it has a value
 * (the forms rule). An empty value is never a format error.
 */
export function correctionError(name: OverrideName, raw: string, today: string): string | undefined {
  const value = raw.trim();
  if (value === "") return undefined;
  switch (name) {
    case "curp":
      return isCurpShape(value) ? undefined : "La CURP lleva 18 caracteres con la forma oficial.";
    case "birthDate":
      return birthDateError(value, today);
    case "homePhone":
    case "mobilePhone":
      return phoneShapeError(value);
    case "email":
      return emailShapeError(value);
    case "postalCode":
      return postalCodeError(value);
    case "bankReference":
      return bankReferenceError(value);
    case "fullName":
      return value.includes(",") ? undefined : "Escríbelo como APELLIDOS, NOMBRES.";
    case "street":
    case "municipality":
      return value.length > MAX_ADDRESS ? "Máximo 120 caracteres." : undefined;
    default:
      return undefined;
  }
}

/** The preview's tabs the review adds. */
export type ReviewTab = "draft" | "omitted";

export function inReviewTab(row: ImportRow, tab: ReviewTab): boolean {
  return tab === "draft" ? row.draft === true : row.reason === "omitted";
}

export function reviewCounts(rows: readonly ImportRow[]): { drafts: number; omitted: number } {
  return { drafts: rows.filter((r) => inReviewTab(r, "draft")).length, omitted: rows.filter((r) => inReviewTab(r, "omitted")).length };
}

/** A row the office may act on: it has issues, may be a draft, or was already decided. */
export function needsReview(row: ImportRow, decision?: RowDecision): boolean {
  return (row.issues ?? []).length > 0 || row.canDraft === true || (decision !== undefined && !isPlainImport(decision));
}

"use client";

import type { ReactElement, ReactNode } from "react";

import { FieldWarning } from "../../_design/form";
import type { IssueField } from "../../studentImport/api";
import { ISSUE_FIELD_LABEL, issueOutcome, issueText } from "../../studentImport/copy";
import type { StudentImportIssue } from "../../students/importIssues";
import type { ImportIssuesState } from "./useImportIssues";

/** What a draft's CURP field says instead of its import issue. */
export const DRAFT_CURP_WARNING = "Falta la CURP: sin ella no se puede inscribir";

/** "Teléfono incompleto. En el Excel decía «55 12»; no se guardó." — every issue of the field. */
export function warningText(issues: readonly StudentImportIssue[]): string | undefined {
  if (issues.length === 0) return undefined;
  return issues
    .map((i) => {
      const outcome = issueOutcome(i.originalValue, i.storedValue);
      return outcome === "" ? `${issueText(i.code)}.` : `${issueText(i.code)}. ${outcome}`;
    })
    .join(" ");
}

/** «Está bien así»: a ≥ 44 px target, named by its field. */
export function AcceptButton({ label, onAccept }: { label: string; onAccept: () => void }): ReactElement {
  return (
    <button
      type="button"
      onClick={onAccept}
      aria-label={`Está bien así: ${label}`}
      style={{ justifySelf: "start", minHeight: 44, padding: "0 14px", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 13.5, fontWeight: 600, cursor: "pointer" }}
    >
      Está bien así
    </button>
  );
}

/**
 * The `warning` and `warningAction` props of one form field for its import
 * issues; empty when it has none (so the flag OFF spreads nothing).
 */
export function fieldWarning(review: ImportIssuesState | undefined, field: IssueField, label: string): { warning?: string; warningAction?: ReactNode } {
  if (review === undefined) return {};
  const warning = warningText(review.of(field));
  if (warning === undefined) return {};
  return { warning, warningAction: <AcceptButton label={label} onAccept={() => void review.accept(field)} /> };
}

/** Values with no single field on the page (a dropped phone): said above where they would go. */
export function StandaloneWarnings({ review, fields }: { review: ImportIssuesState | undefined; fields: readonly IssueField[] }): ReactElement | null {
  if (review === undefined) return null;
  const shown = fields.filter((f) => review.of(f).length > 0);
  if (shown.length === 0) return null;
  return (
    <div style={{ display: "grid", gap: 8 }}>
      {shown.map((f) => {
        const label = ISSUE_FIELD_LABEL[f];
        return <FieldWarning key={f} text={`${label}: ${warningText(review.of(f)) ?? ""}`} action={<AcceptButton label={label} onAccept={() => void review.accept(f)} />} />;
      })}
    </div>
  );
}

"use client";

import { useId, type MouseEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { API_URL } from "../../api-url";
import type { StudentImportFamilies } from "../../studentImport/api";
import { builtFamilyNames, canUndoFamilies, familiesHeadline, reviewLinkText, undoOutcome } from "../../studentImport/families";
import { alertStyle, cardStyle, noteStyle } from "./parts";
import { useImportFamiliesUndo } from "./useImportFamiliesUndo";

/** More names than this go behind «Ver familias». */
const NAMES_SHOWN_OPEN = 5;

/** Where the suggestions left to review live (Alumnos › Familias). */
export interface FamiliesReviewLink {
  href: string;
  onOpen: () => void;
}

export interface ImportFamiliesProps {
  families: StudentImportFamilies;
  /** The commit's batch; without it there is nothing to undo against. */
  batchId: string | undefined;
  review: FamiliesReviewLink;
}

const LINK = { display: "inline-flex", alignItems: "center", minHeight: 44, fontSize: 15, fontWeight: 600, color: "var(--link)", textDecoration: "underline" } as const;

/**
 * The families the import built on its own (PLAN-familias-importacion, owner
 * decision "Armarlas solas"), with Deshacer. Reversible, so no dialog; the
 * outcome stays on the step until the office leaves it.
 */
export function ImportFamilies({ families, batchId, review }: ImportFamiliesProps): ReactElement {
  const titleId = useId();
  const names = builtFamilyNames(families);
  const toReview = families.toReview ?? 0;
  const undoable = batchId !== undefined && canUndoFamilies(families);

  return (
    <section aria-labelledby={titleId} style={cardStyle}>
      <h3 id={titleId} style={{ fontSize: 16, fontWeight: 700, color: "var(--text)", margin: 0 }}>Familias</h3>
      <p style={{ fontSize: 15, color: "var(--text)", margin: 0 }}>{familiesHeadline(families)}</p>
      <FamilyNames names={names} />
      {toReview > 0 && (
        <a
          href={review.href}
          onClick={(e: MouseEvent<HTMLAnchorElement>) => {
            e.preventDefault();
            review.onOpen();
          }}
          style={LINK}
        >
          {reviewLinkText(toReview)}<span aria-hidden="true">&nbsp;→</span>
        </a>
      )}
      {undoable && <UndoFamilies batchId={batchId} />}
    </section>
  );
}

function FamilyNames({ names }: { names: readonly string[] }): ReactElement | null {
  if (names.length === 0) return null;
  const list = (
    <ul style={{ display: "flex", flexWrap: "wrap", gap: "4px 14px", listStyle: "none", margin: 0, padding: 0, fontSize: 14.5, color: "var(--text-muted)" }}>
      {names.map((n, i) => <li key={`${n}-${i}`}>{n}</li>)}
    </ul>
  );
  if (names.length <= NAMES_SHOWN_OPEN) return list;
  return (
    <details>
      <summary style={{ cursor: "pointer", minHeight: 44, display: "flex", alignItems: "center", fontSize: 15, fontWeight: 600, color: "var(--text)" }}>Ver familias</summary>
      {list}
    </details>
  );
}

function UndoFamilies({ batchId }: { batchId: string }): ReactElement {
  const undo = useImportFamiliesUndo(API_URL, batchId);
  const lines = undo.outcome === undefined ? [] : undoOutcome(undo.outcome);
  return (
    <>
      <div>
        <Btn kind="ghost" busy={undo.busy} onClick={() => void undo.undo()} style={{ height: 44 }}>Deshacer</Btn>
      </div>
      <div role="status" style={lines.length === 0 ? undefined : noteStyle}>
        {lines.map((line) => <p key={line} style={{ margin: 0 }}>{line}</p>)}
      </div>
      {undo.error !== undefined && <p role="alert" style={alertStyle}>{undo.error}</p>}
    </>
  );
}

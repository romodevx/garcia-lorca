"use client";

import { useId, type MouseEvent, type ReactElement, type ReactNode } from "react";

import { Btn } from "../../_design/Controls";
import { API_URL } from "../../api-url";
import type { StudentImportFamilies } from "../../studentImport/api";
import {
  UNDONE_HEADLINE,
  builtFamilyNames,
  canUndoFamilies,
  familiesHeadline,
  reviewLinkText,
  undoIsComplete,
  undoOutcome,
  type NamedFamily,
} from "../../studentImport/families";
import { alertStyle, cardStyle, noteStyle } from "./parts";
import { useImportFamiliesUndo } from "./useImportFamiliesUndo";

/** More names than this go behind «Ver familias». */
const NAMES_SHOWN_OPEN = 5;

/** Alumnos › Familias: the suggestions left to review, and each family's own page. */
export interface FamiliesReviewLink {
  href: string;
  onOpen: () => void;
  familyHref: (familyId: string) => string;
  onOpenFamily: (familyId: string) => void;
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
  const undo = useImportFamiliesUndo(API_URL, batchId);
  // Everything taken back: the step stops saying "Armamos" and offers nothing more to undo.
  const undone = undoIsComplete(undo.outcome);
  const toReview = families.toReview ?? 0;
  const undoable = !undone && batchId !== undefined && canUndoFamilies(families);
  const lines = undo.outcome === undefined ? [] : undoOutcome(undo.outcome);

  return (
    <section aria-labelledby={titleId} style={cardStyle}>
      <h3 id={titleId} style={{ fontSize: 16, fontWeight: 700, color: "var(--text)", margin: 0 }}>Familias</h3>
      <p style={{ fontSize: 15, color: "var(--text)", margin: 0 }}>{undone ? UNDONE_HEADLINE : familiesHeadline(families)}</p>
      {!undone && <FamilyNames families={builtFamilyNames(families)} review={review} />}
      {toReview > 0 && (
        <InAppLink href={review.href} onOpen={review.onOpen}>
          {reviewLinkText(toReview)}<span aria-hidden="true">&nbsp;→</span>
        </InAppLink>
      )}
      {undoable && (
        <div>
          <Btn kind="ghost" busy={undo.busy} onClick={() => void undo.undo()} style={{ height: 44 }}>Deshacer</Btn>
        </div>
      )}
      <div role="status" style={lines.length === 0 ? undefined : noteStyle}>
        {lines.map((line) => <p key={line} style={{ margin: 0 }}>{line}</p>)}
      </div>
      {undo.error !== undefined && <p role="alert" style={alertStyle}>{undo.error}</p>}
    </section>
  );
}

/** A real link (open in a new tab works) that navigates in-app on a plain click. */
function InAppLink({ href, onOpen, children }: { href: string; onOpen: () => void; children: ReactNode }): ReactElement {
  const click = (e: MouseEvent<HTMLAnchorElement>): void => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    onOpen();
  };
  return <a href={href} onClick={click} style={LINK}>{children}</a>;
}

function FamilyNames({ families, review }: { families: readonly NamedFamily[]; review: FamiliesReviewLink }): ReactElement | null {
  if (families.length === 0) return null;
  const list = (
    <ul style={{ display: "flex", flexWrap: "wrap", gap: "0 14px", listStyle: "none", margin: 0, padding: 0 }}>
      {families.map((f) => (
        <li key={f.id}>
          <InAppLink href={review.familyHref(f.id)} onOpen={() => review.onOpenFamily(f.id)}>{f.name}</InAppLink>
        </li>
      ))}
    </ul>
  );
  if (families.length <= NAMES_SHOWN_OPEN) return list;
  return (
    <details>
      <summary style={{ cursor: "pointer", minHeight: 44, display: "flex", alignItems: "center", fontSize: 15, fontWeight: 600, color: "var(--text)" }}>Ver familias</summary>
      {list}
    </details>
  );
}

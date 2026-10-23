"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { FamilySuggestion } from "../../families/api";
import { confidenceLabel, reasonLabel } from "../../families/model";
import type { FamilyPupil } from "./familyPupil";

const CHIP = { display: "inline-flex", alignItems: "center", height: 26, padding: "0 10px", borderRadius: 99, fontSize: 12.5, fontWeight: 700 } as const;
const ACTION = { height: 44, padding: "0 16px", fontSize: 14.5 } as const;

export interface SuggestionActions {
  /** Absent without guardians:write: building a family hands out access. */
  onAccept?: () => void;
  onAdjust?: () => void;
  /** Absent without students:update. */
  onDismiss?: () => void;
}

/** One group of pupils that look like siblings, and the three answers to it. */
export function SuggestionCard({ suggestion, pupils, busy, error, actions }: {
  suggestion: FamilySuggestion;
  pupils: ReadonlyMap<string, FamilyPupil>;
  busy: boolean;
  error?: string;
  actions: SuggestionActions;
}): ReactElement {
  const high = suggestion.confidence === "high";
  return (
    <article aria-label={`Sugerencia ${suggestion.proposedName}`} aria-busy={busy || undefined} style={{ display: "grid", gap: 10, padding: "16px 18px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <h3 style={{ fontSize: 16, fontWeight: 700, color: "var(--text)", flex: 1, minWidth: 0 }}>{suggestion.proposedName}</h3>
        <span style={{ ...CHIP, background: high ? "var(--ok-bg)" : "var(--warn-bg)", color: high ? "var(--ok-fg)" : "var(--warn-fg)" }}>{confidenceLabel(suggestion.confidence)}</span>
      </div>
      <ul style={{ display: "grid", gap: 4, listStyle: "none", margin: 0, padding: 0 }}>
        {suggestion.studentIds.map((id) => {
          const p = pupils.get(id);
          return (
            <li key={id} style={{ display: "flex", gap: 8, flexWrap: "wrap", fontSize: 14.5 }}>
              <span style={{ fontWeight: 600, color: "var(--text)" }}>{p?.name ?? "Alumno"}</span>
              {p !== undefined && <span style={{ color: "var(--text-faint)" }}>{p.room}</span>}
            </li>
          );
        })}
      </ul>
      <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
        {suggestion.reasons.map((r) => (
          <span key={r} style={{ ...CHIP, background: "var(--surface-3)", color: "var(--text-muted)" }}>{reasonLabel(r)}</span>
        ))}
      </div>
      {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{error}</p>}
      {(actions.onAccept !== undefined || actions.onDismiss !== undefined) && (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          {actions.onAccept !== undefined && <Btn icon="check" busy={busy} onClick={actions.onAccept} style={ACTION}>Es familia</Btn>}
          {actions.onAdjust !== undefined && <Btn kind="ghost" onClick={actions.onAdjust} style={ACTION}>Ajustar</Btn>}
          {actions.onDismiss !== undefined && <Btn kind="ghost" onClick={actions.onDismiss} style={ACTION}>No es familia</Btn>}
        </div>
      )}
    </article>
  );
}

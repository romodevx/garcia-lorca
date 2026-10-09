"use client";

import { useState, type CSSProperties, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import type { Term } from "../../config/api";
import { unassignedMonths, type MonthAssignment } from "../../config/termMonths";
import { monthChips, type MonthChip } from "../../_lib/monthChips";
import { ErrorAlert, SelectChip } from "./parts";
import { useSetupPresentation } from "./setup/SetupPresentation";

export interface TermMonthsEditorProps {
  terms: readonly Term[];
  /** The cycle's months, oldest first. */
  months: readonly string[];
  assignment: MonthAssignment;
  error: string | undefined;
  onTap: (termId: string, month: string) => void;
}

type View = "grid" | "list";

const TH: CSSProperties = { padding: "8px 6px", fontSize: 12.5, fontWeight: 700, letterSpacing: ".04em", textTransform: "uppercase", color: "var(--text-faint)", whiteSpace: "nowrap" };

function cellStyle(on: boolean): CSSProperties {
  return {
    width: 44,
    height: 44,
    borderRadius: "var(--r-md)",
    display: "grid",
    placeItems: "center",
    border: `1.5px solid ${on ? "var(--forest)" : "var(--border)"}`,
    background: on ? "var(--forest)" : "var(--surface)",
    color: on ? "#fff" : "var(--text-faint)",
    cursor: "pointer",
  };
}

/**
 * Which months each term grades, as a months × terms grid: one tap assigns a
 * month, a tap on another row moves it. The grid scrolls sideways inside its
 * own box (never the page); the list view gives each term its months as
 * toggle chips, for a phone or a screen reader.
 */
export function TermMonthsEditor({ terms, months, assignment, error, onTap }: TermMonthsEditorProps): ReactElement {
  // A compact setup step has no room for the grid: it shows the list, never scrolling sideways (§1.6).
  // Derived on every render, so a window resized after mount follows; the user's pick wins once made.
  const { compact } = useSetupPresentation();
  const [picked, setView] = useState<View | undefined>(undefined);
  const view: View = picked ?? (compact ? "list" : "grid");
  const chips = monthChips(months);
  const has = (termId: string, month: string): boolean => assignment[termId]?.includes(month) === true;
  const free = new Set(unassignedMonths(months, assignment));

  return (
    <div style={{ display: "grid", gap: 12, marginBottom: 16 }}>
      <div role="group" aria-label="Vista" style={{ display: "flex", gap: 8 }}>
        <SelectChip selected={view === "grid"} onClick={() => setView("grid")}>Tabla</SelectChip>
        <SelectChip selected={view === "list"} onClick={() => setView("list")}>Lista</SelectChip>
      </div>

      {view === "grid" ? (
        <div style={{ overflowX: "auto", contain: "inline-size", border: "1px solid var(--border)", borderRadius: "var(--r-md)" }}>
          <table aria-label="Meses de cada periodo" style={{ borderCollapse: "separate", borderSpacing: 4, padding: 4 }}>
            <thead>
              <tr>
                <th scope="col" style={{ ...TH, textAlign: "left" }}>Periodo</th>
                {chips.map((c) => <th key={c.month} scope="col" title={c.fullName} style={TH}>{c.label}</th>)}
              </tr>
            </thead>
            <tbody>
              {terms.map((t) => (
                <tr key={t.id}>
                  <th scope="row" style={{ textAlign: "left", padding: "0 10px 0 4px", fontSize: 14, fontWeight: 600, color: "var(--text)", whiteSpace: "nowrap" }}>{t.name}</th>
                  {chips.map((c) => (
                    <td key={c.month}>
                      <button type="button" aria-pressed={has(t.id, c.month)} aria-label={`${c.fullName} en ${t.name}`} onClick={() => onTap(t.id, c.month)} style={cellStyle(has(t.id, c.month))}>
                        {has(t.id, c.month) && <Icon name="check" size={18} stroke={2.6} />}
                      </button>
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div style={{ display: "grid", gap: 12 }}>
          {terms.map((t) => <TermChipRow key={t.id} term={t} chips={chips} has={has} onTap={onTap} />)}
        </div>
      )}

      {free.size > 0 && (
        <p style={{ fontSize: 13.5, color: "var(--warn-fg)" }}>
          Sin periodo: {chips.filter((c) => free.has(c.month)).map((c) => c.label).join(", ")}
        </p>
      )}
      {error !== undefined && <ErrorAlert message={error} />}
    </div>
  );
}

function TermChipRow({ term, chips, has, onTap }: {
  term: Term;
  chips: readonly MonthChip[];
  has: (termId: string, month: string) => boolean;
  onTap: (termId: string, month: string) => void;
}): ReactElement {
  return (
    <div>
      <p style={{ fontSize: 14, fontWeight: 700, color: "var(--text)", marginBottom: 6 }}>{term.name}</p>
      <div role="group" aria-label={`Meses de ${term.name}`} style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
        {chips.map((c) => (
          <button
            key={c.month}
            type="button"
            aria-pressed={has(term.id, c.month)}
            aria-label={c.fullName}
            onClick={() => onTap(term.id, c.month)}
            style={{
              minHeight: 44, minWidth: 52, padding: "0 12px", borderRadius: "var(--r-pill)", fontSize: 14, fontWeight: 600, cursor: "pointer",
              border: `1.5px solid ${has(term.id, c.month) ? "var(--forest)" : "var(--border)"}`,
              background: has(term.id, c.month) ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
              color: has(term.id, c.month) ? "var(--forest)" : "var(--text-muted)",
            }}
          >
            {c.label}
          </button>
        ))}
      </div>
    </div>
  );
}

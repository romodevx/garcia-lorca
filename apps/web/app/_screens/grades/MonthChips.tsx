"use client";

import { type ReactElement } from "react";

import { monthChips } from "../../grades/monthly";

export interface MonthChipsProps {
  months: readonly string[];
  selected: string;
  onChoose: (month: string) => void;
}

/** The cycle's months as one row of pills; it scrolls sideways on its own, never the page. */
export function MonthChips({ months, selected, onChoose }: MonthChipsProps): ReactElement {
  return (
    <div role="group" aria-label="Mes" style={{ display: "flex", gap: 8, overflowX: "auto", padding: "2px 2px 6px", marginBottom: 14, maxWidth: "100%" }}>
      {monthChips(months).map((chip) => {
        const active = chip.month === selected;
        return (
          <button
            key={chip.month}
            type="button"
            aria-pressed={active}
            title={chip.fullName}
            onClick={() => onChoose(chip.month)}
            style={{
              flexShrink: 0,
              minHeight: 44,
              minWidth: 56,
              padding: "0 16px",
              borderRadius: "var(--r-pill)",
              border: `1.5px solid ${active ? "var(--forest)" : "var(--border)"}`,
              background: active ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
              color: "var(--text)",
              fontSize: 16,
              fontWeight: active ? 700 : 600,
              cursor: "pointer",
            }}
          >
            {chip.label}
          </button>
        );
      })}
    </div>
  );
}

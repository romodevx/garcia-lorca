"use client";

import { type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { periodChips, type PeriodChip, type PeriodSheet } from "../../grades/periodSheet";

/** Said on a period that has not started — the server's own words for the refusal. */
export const NOT_STARTED_HINT = "Aún no empieza";
/** Said on a period whose term the director closed. */
export const CLOSED_HINT = "Cerrado";
/** Said on the period that contains today. */
export const CURRENT_HINT = "En curso";

export interface PeriodChipsProps {
  sheet: Pick<PeriodSheet, "periods" | "today" | "closedPeriodIds" | "currentPeriodId">;
  selected: string;
  onChoose: (periodId: string) => void;
}

function hintOf(chip: PeriodChip): string | undefined {
  if (chip.closed) return CLOSED_HINT;
  if (chip.notStarted) return NOT_STARTED_HINT;
  return chip.current ? CURRENT_HINT : undefined;
}

/**
 * The grade's evaluation periods as one row of pills; it scrolls sideways on
 * its own, never the page. A closed period stays selectable (read-only); one
 * that has not started cannot be chosen yet.
 */
export function PeriodChips({ sheet, selected, onChoose }: PeriodChipsProps): ReactElement {
  return (
    <div role="group" aria-label="Periodo" style={{ display: "flex", gap: 8, overflowX: "auto", padding: "2px 2px 6px", marginBottom: 14, maxWidth: "100%" }}>
      {periodChips(sheet).map((chip) => {
        const active = chip.id === selected;
        const hint = hintOf(chip);
        return (
          <button
            key={chip.id}
            type="button"
            aria-pressed={active}
            disabled={chip.notStarted}
            data-not-started={chip.notStarted ? "true" : undefined}
            data-closed={chip.closed ? "true" : undefined}
            data-current={chip.current ? "true" : undefined}
            onClick={() => onChoose(chip.id)}
            style={{
              flexShrink: 0,
              minHeight: 44,
              padding: "6px 16px",
              borderRadius: "var(--r-lg)",
              border: `1.5px ${chip.notStarted ? "dashed" : "solid"} ${active ? "var(--forest)" : "var(--border)"}`,
              background: active ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
              display: "inline-flex",
              flexDirection: "column",
              alignItems: "flex-start",
              textAlign: "left",
              color: chip.notStarted || chip.closed ? "var(--text-muted)" : "var(--text)",
              cursor: chip.notStarted ? "not-allowed" : "pointer",
            }}
          >
            <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 16, fontWeight: active ? 700 : 600 }}>
              {chip.closed && <Icon name="lock" size={16} />}
              {chip.label}
            </span>
            <span style={{ fontSize: 12.5, fontWeight: 500, color: "var(--text-muted)" }}>
              {` ${chip.range}`}
              {hint !== undefined && <span style={{ fontWeight: 700, color: chip.current ? "var(--ok-fg)" : "var(--warn-fg)" }}>{` · ${hint}`}</span>}
            </span>
          </button>
        );
      })}
    </div>
  );
}

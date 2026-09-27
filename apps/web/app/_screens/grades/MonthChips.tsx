"use client";

import { type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { monthChips } from "../../grades/monthly";

/** Said on a month that has not come yet — the server's own words for the refusal. */
export const FUTURE_MONTH_REASON = "Aún no llega ese mes";

export interface MonthChipsProps {
  months: readonly string[];
  selected: string;
  /** The server's current month; the later months carry a lock (still selectable, read-only). */
  currentMonth?: string;
  onChoose: (month: string) => void;
}

/** The cycle's months as one row of pills; it scrolls sideways on its own, never the page. */
export function MonthChips({ months, selected, currentMonth, onChoose }: MonthChipsProps): ReactElement {
  return (
    <div role="group" aria-label="Mes" style={{ display: "flex", gap: 8, overflowX: "auto", padding: "2px 2px 6px", marginBottom: 14, maxWidth: "100%" }}>
      {monthChips(months, currentMonth).map((chip) => {
        const active = chip.month === selected;
        return (
          <button
            key={chip.month}
            type="button"
            aria-pressed={active}
            title={chip.future ? `${chip.fullName} · ${FUTURE_MONTH_REASON}` : chip.fullName}
            data-future={chip.future ? "true" : undefined}
            onClick={() => onChoose(chip.month)}
            style={{
              flexShrink: 0,
              minHeight: 44,
              minWidth: 56,
              padding: "0 16px",
              borderRadius: "var(--r-pill)",
              border: `1.5px ${chip.future ? "dashed" : "solid"} ${active ? "var(--forest)" : "var(--border)"}`,
              background: active ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
              display: "inline-flex",
              alignItems: "center",
              gap: 6,
              color: chip.future ? "var(--text-muted)" : "var(--text)",
              fontSize: 16,
              fontWeight: active ? 700 : 600,
              cursor: "pointer",
            }}
          >
            {chip.future && <Icon name="lock" size={16} />}
            {chip.label}
          </button>
        );
      })}
    </div>
  );
}

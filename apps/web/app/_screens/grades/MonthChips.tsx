"use client";

import { type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { monthChips, type MonthChip } from "../../grades/monthly";

/** Said on a month that has not come yet — the server's own words for the refusal. */
export const FUTURE_MONTH_REASON = "Aún no llega ese mes";
/** Said on a month whose period the director closed. */
export const CLOSED_MONTH_REASON = "Periodo cerrado";
/** Said on a month no period grades: it can be captured, but no boleta counts it. */
export const UNMAPPED_MONTH_HINT = "Sin periodo";

export interface MonthChipsProps {
  months: readonly string[];
  selected: string;
  /** The server's current month; the later months carry a lock (still selectable, read-only). */
  currentMonth?: string;
  /** Months of a closed period; they carry a lock too (still selectable, read-only). */
  closedMonths?: readonly string[];
  /** Months no period grades; they carry a "Sin periodo" hint (still capturable). */
  unmappedMonths?: readonly string[];
  onChoose: (month: string) => void;
}

/** The cycle's months as one row of pills; it scrolls sideways on its own, never the page. */
function chipTitle(chip: MonthChip): string {
  if (chip.closed) return `${chip.fullName} · ${CLOSED_MONTH_REASON}`;
  if (chip.future) return `${chip.fullName} · ${FUTURE_MONTH_REASON}`;
  return chip.unmapped ? `${chip.fullName} · ${UNMAPPED_MONTH_HINT}` : chip.fullName;
}

export function MonthChips({ months, selected, currentMonth, closedMonths, unmappedMonths, onChoose }: MonthChipsProps): ReactElement {
  return (
    <div role="group" aria-label="Mes" style={{ display: "flex", gap: 8, overflowX: "auto", padding: "2px 2px 6px", marginBottom: 14, maxWidth: "100%" }}>
      {monthChips(months, currentMonth, closedMonths, unmappedMonths).map((chip) => {
        const active = chip.month === selected;
        const locked = chip.future || chip.closed;
        return (
          <button
            key={chip.month}
            type="button"
            aria-pressed={active}
            title={chipTitle(chip)}
            data-future={chip.future ? "true" : undefined}
            data-closed={chip.closed ? "true" : undefined}
            data-unmapped={chip.unmapped ? "true" : undefined}
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
              color: locked ? "var(--text-muted)" : "var(--text)",
              fontSize: 16,
              fontWeight: active ? 700 : 600,
              cursor: "pointer",
            }}
          >
            {locked && <Icon name="lock" size={16} />}
            {chip.label}
            {chip.unmapped && <span style={{ fontSize: 12.5, fontWeight: 600, color: "var(--warn-fg)" }}>{`· ${UNMAPPED_MONTH_HINT}`}</span>}
          </button>
        );
      })}
    </div>
  );
}

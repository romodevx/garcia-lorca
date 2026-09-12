"use client";

import type { ReactElement } from "react";
import type { AttendanceStatus } from "../../attendance/api";

/**
 * Presentational primitives for the attendance surface. The catalog primitives
 * (Section/SelectField/DateField/EmptyHint/ErrorAlert) are re-exported so a panel
 * imports everything visual from one place.
 */

export { DateField, SelectField, type SelectOption } from "../../_design/form";
export { EmptyHint, ErrorAlert, Section } from "../config/parts";

/** Per-status tone: present is the calm default, the exceptions stand out. */
const TONE: Record<AttendanceStatus, string> = {
  present: "var(--forest)",
  absent: "var(--red)",
  late: "var(--orange)",
  excused: "var(--purple)",
};

export interface StatusOption {
  status: AttendanceStatus;
  label: string;
}

/**
 * The one-click status control of a pass-list row: a segmented radio group where
 * "present" is already selected, so taking a full group costs zero clicks and one
 * click per exception. Buttons (not native radios) keep the segments visually
 * joined while `role="radio"` + `aria-checked` keep the semantics — and the
 * screen-reader/test name comes from the group's label ("Asistencia de …").
 */
export function StatusPicker({
  label,
  options,
  value,
  onChange,
}: {
  label: string;
  options: readonly StatusOption[];
  value: AttendanceStatus;
  onChange: (status: AttendanceStatus) => void;
}): ReactElement {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      style={{
        display: "inline-flex",
        gap: 4,
        padding: 4,
        background: "var(--surface-3)",
        border: "1px solid var(--border)",
        borderRadius: "var(--r-pill)",
      }}
    >
      {options.map((option) => {
        const on = option.status === value;
        const tone = TONE[option.status];
        return (
          <button
            key={option.status}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => onChange(option.status)}
            style={{
              height: 34,
              padding: "0 14px",
              borderRadius: 99,
              fontSize: 13.5,
              fontWeight: 600,
              whiteSpace: "nowrap",
              background: on ? `color-mix(in oklab, ${tone} 14%, var(--surface))` : "transparent",
              border: `1.5px solid ${on ? tone : "transparent"}`,
              color: on ? tone : "var(--text-muted)",
              transition: "all .15s var(--ease)",
            }}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}

export interface CounterDef {
  status: AttendanceStatus;
  label: string;
  count: number;
}

/** Live tally of the day, so the teacher sees the exceptions before saving. */
export function StatusCounters({ counters }: { counters: readonly CounterDef[] }): ReactElement {
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 10 }}>
      {counters.map((counter) => (
        <span
          key={counter.status}
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: 8,
            padding: "8px 14px",
            borderRadius: "var(--r-pill)",
            background: "var(--surface)",
            border: "1px solid var(--border)",
            fontSize: 13.5,
            color: "var(--text-muted)",
            fontWeight: 600,
          }}
        >
          <span style={{ width: 9, height: 9, borderRadius: 99, background: TONE[counter.status] }} />
          {counter.label}
          <strong style={{ color: "var(--text)", fontFamily: "var(--font-display)" }}>{counter.count}</strong>
        </span>
      ))}
    </div>
  );
}

"use client";

import type { CSSProperties, ReactElement, ReactNode } from "react";

/**
 * Presentational primitives for the admin surface. The catalog panels reuse the
 * config primitives (Section/TextField/ErrorAlert/EmptyHint) — re-exported here
 * so a panel imports everything from one place — plus the checklist controls the
 * role editor and user role-picker need. Kept dumb (no data fetching); every
 * control is labelled for a11y + testability.
 */

export { EmptyHint, ErrorAlert, Section, SelectField, TextField } from "../config/parts";

const legendStyle: CSSProperties = {
  fontSize: 13,
  fontWeight: 600,
  color: "var(--text-muted)",
  marginBottom: 8,
};

export interface CheckOption {
  value: string;
  label: string;
}

/**
 * An accessible multi-select checklist rendered as a `<fieldset>` with a
 * `<legend>`; `groups` optionally clusters options under sub-headings (used by
 * the permission catalog). Each checkbox carries an `aria-label` so tests select
 * it by its human label. Controlled via `selected` + `onToggle`.
 */
export function CheckList({
  legend,
  options,
  groups,
  selected,
  onToggle,
  disabled = false,
}: {
  legend: string;
  options?: readonly CheckOption[];
  groups?: readonly { heading: string; options: readonly CheckOption[] }[];
  selected: ReadonlySet<string>;
  onToggle: (value: string) => void;
  disabled?: boolean;
}): ReactElement {
  const renderOption = (opt: CheckOption): ReactElement => {
    const on = selected.has(opt.value);
    return (
      <label
        key={opt.value}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 9,
          padding: "7px 12px",
          borderRadius: "var(--r-md)",
          cursor: disabled ? "not-allowed" : "pointer",
          border: `1.5px solid ${on ? "var(--forest)" : "var(--border)"}`,
          background: on ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
          color: on ? "var(--forest)" : "var(--text-muted)",
          fontSize: 13.5,
          fontWeight: 600,
        }}
      >
        <input
          type="checkbox"
          aria-label={opt.label}
          checked={on}
          disabled={disabled}
          onChange={() => onToggle(opt.value)}
          style={{ accentColor: "var(--forest)" }}
        />
        {opt.label}
      </label>
    );
  };

  return (
    <fieldset style={{ border: "none", padding: 0, margin: 0 }}>
      <legend style={legendStyle}>{legend}</legend>
      {groups !== undefined
        ? groups.map((g) => (
            <div key={g.heading} style={{ marginBottom: 10 }}>
              <p style={{ fontSize: 12, fontWeight: 700, color: "var(--text-faint)", margin: "0 0 6px", textTransform: "uppercase", letterSpacing: 0.4 }}>
                {g.heading}
              </p>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>{g.options.map(renderOption)}</div>
            </div>
          ))
        : (
            <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>{(options ?? []).map(renderOption)}</div>
          )}
    </fieldset>
  );
}

/** A small status pill for the users list (active / suspended). */
export function StatusPill({ status }: { status: string }): ReactElement {
  const active = status === "active";
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        padding: "2px 10px",
        borderRadius: "var(--r-pill)",
        fontSize: 12,
        fontWeight: 700,
        background: active ? "var(--good-bg, color-mix(in oklab, var(--forest) 14%, var(--surface)))" : "var(--bad-bg)",
        color: active ? "var(--forest)" : "var(--bad-fg)",
      }}
    >
      {active ? "Activo" : "Suspendido"}
    </span>
  );
}

export function FieldLabel({ children }: { children: ReactNode }): ReactElement {
  return <span style={legendStyle}>{children}</span>;
}

"use client";

import type { ChangeEvent, CSSProperties, ReactElement, ReactNode } from "react";

/**
 * Presentational primitives shared by the catalog panels. Kept dumb (no data
 * fetching) so each panel stays small and the design system (CSS tokens) is the
 * single source of visual truth. Every input is labelled for a11y + testability.
 */

const inputStyle: CSSProperties = {
  height: 44,
  padding: "0 14px",
  borderRadius: "var(--r-md)",
  border: "1px solid var(--border)",
  background: "var(--surface)",
  color: "var(--text)",
  fontSize: 15,
  outline: "none",
  width: "100%",
};

const labelStyle: CSSProperties = {
  display: "block",
  fontSize: 13,
  fontWeight: 600,
  color: "var(--text-muted)",
  marginBottom: 6,
};

export function Section({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
}): ReactElement {
  return (
    <section
      style={{
        background: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--r-xl)",
        boxShadow: "var(--shadow-card)",
        padding: "20px 22px",
      }}
    >
      <header style={{ marginBottom: 16 }}>
        <h2 style={{ fontSize: 18, fontWeight: 700, color: "var(--text)" }}>{title}</h2>
        {subtitle !== undefined && <p style={{ fontSize: 13.5, color: "var(--text-muted)", marginTop: 2 }}>{subtitle}</p>}
      </header>
      {children}
    </section>
  );
}

export function TextField({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}): ReactElement {
  return (
    <label style={{ display: "block" }}>
      <span style={labelStyle}>{label}</span>
      <input
        aria-label={label}
        value={value}
        placeholder={placeholder}
        onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        style={inputStyle}
      />
    </label>
  );
}

export function NumberField({
  label,
  value,
  onChange,
  min = 0,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  min?: number;
}): ReactElement {
  return (
    <label style={{ display: "block" }}>
      <span style={labelStyle}>{label}</span>
      <input
        type="number"
        aria-label={label}
        min={min}
        value={Number.isNaN(value) ? "" : value}
        onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(Number(e.target.value))}
        style={inputStyle}
      />
    </label>
  );
}

export function DateField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}): ReactElement {
  return (
    <label style={{ display: "block" }}>
      <span style={labelStyle}>{label}</span>
      <input
        type="date"
        aria-label={label}
        value={value}
        onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        style={inputStyle}
      />
    </label>
  );
}

export interface SelectOption {
  value: string;
  label: string;
}

export function SelectField({
  label,
  value,
  options,
  onChange,
  placeholder,
  disabled = false,
}: {
  label: string;
  value: string;
  options: readonly SelectOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
}): ReactElement {
  return (
    <label style={{ display: "block" }}>
      <span style={labelStyle}>{label}</span>
      <select
        aria-label={label}
        value={value}
        disabled={disabled}
        onChange={(e: ChangeEvent<HTMLSelectElement>) => onChange(e.target.value)}
        style={{ ...inputStyle, cursor: disabled ? "not-allowed" : "pointer", opacity: disabled ? 0.6 : 1 }}
      >
        <option value="">{placeholder ?? "Selecciona…"}</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  );
}

export interface RadioOption {
  value: string;
  label: string;
}

export function RadioGroup({
  legend,
  options,
  value,
  onChange,
}: {
  legend: string;
  options: readonly RadioOption[];
  value: string;
  onChange: (value: string) => void;
}): ReactElement {
  return (
    <fieldset style={{ border: "none", padding: 0, margin: 0 }}>
      <legend style={labelStyle}>{legend}</legend>
      <div style={{ display: "flex", gap: 8 }}>
        {options.map((opt) => {
          const on = value === opt.value;
          return (
            <label
              key={opt.value}
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 8,
                height: 44,
                padding: "0 14px",
                borderRadius: "var(--r-pill)",
                cursor: "pointer",
                border: `1.5px solid ${on ? "var(--forest)" : "var(--border)"}`,
                background: on ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
                color: on ? "var(--forest)" : "var(--text-muted)",
                fontWeight: 600,
                fontSize: 14,
              }}
            >
              <input
                type="radio"
                name={legend}
                aria-label={opt.label}
                checked={on}
                onChange={() => onChange(opt.value)}
                style={{ accentColor: "var(--forest)" }}
              />
              {opt.label}
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}

export function ErrorAlert({ message }: { message: string }): ReactElement {
  return (
    <p
      role="alert"
      style={{
        margin: "10px 0 0",
        padding: "10px 14px",
        borderRadius: "var(--r-md)",
        background: "var(--bad-bg)",
        color: "var(--bad-fg)",
        fontSize: 13.5,
        fontWeight: 600,
      }}
    >
      {message}
    </p>
  );
}

export function EmptyHint({ children }: { children: ReactNode }): ReactElement {
  return (
    <p style={{ padding: "10px 0", color: "var(--text-faint)", fontSize: 14 }}>{children}</p>
  );
}

/** A selectable chip row item (level/grade picker). */
export function SelectChip({
  selected,
  onClick,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  children: ReactNode;
}): ReactElement {
  return (
    <button
      onClick={onClick}
      aria-pressed={selected}
      style={{
        height: 40,
        padding: "0 16px",
        borderRadius: "var(--r-pill)",
        fontSize: 14,
        fontWeight: 600,
        border: `1.5px solid ${selected ? "var(--forest)" : "var(--border)"}`,
        background: selected ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
        color: selected ? "var(--forest)" : "var(--text-muted)",
        transition: "all .15s var(--ease)",
      }}
    >
      {children}
    </button>
  );
}

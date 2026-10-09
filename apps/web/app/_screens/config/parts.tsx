"use client";

import type { CSSProperties, ReactElement, ReactNode } from "react";

import { useSetupPresentation } from "./setup/SetupPresentation";

/**
 * Presentational primitives shared by the catalog panels. Kept dumb (no data
 * fetching) so each panel stays small and the design system (CSS tokens) is the
 * single source of visual truth. Every input is labelled for a11y + testability.
 */

const labelStyle: CSSProperties = {
  display: "block",
  fontSize: 13,
  fontWeight: 600,
  color: "var(--text-muted)",
  marginBottom: 6,
};

const cardStyle: CSSProperties = {
  background: "var(--surface)",
  border: "1px solid var(--border)",
  borderRadius: "var(--r-xl)",
  boxShadow: "var(--shadow-card)",
  padding: "20px 22px",
};

/** A panel: a card with its h2 title. Inside a setup step it is bare — the open step is the card. */
export function Section({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
}): ReactElement {
  const { inSetup } = useSetupPresentation();
  return (
    <section style={inSetup ? undefined : cardStyle}>
      <header style={{ marginBottom: 16 }}>
        {/* In a step the h2 is the step's title: opening the step moves focus to it (§1.7). */}
        <h2 tabIndex={inSetup ? -1 : undefined} style={{ fontSize: 18, fontWeight: 700, color: "var(--text)" }}>{title}</h2>
        {subtitle !== undefined && <p style={{ fontSize: 13.5, color: "var(--text-muted)", marginTop: 2 }}>{subtitle}</p>}
      </header>
      {children}
    </section>
  );
}

/*
 * Aquí vivían TextField, NumberField, DateField y SelectField.
 *
 * Se fueron a app/_design/form/, que es donde captura TODO el sistema. Mientras
 * estuvieron escondidos bajo _screens/config/, el resto de las pantallas no los
 * encontró y dibujó sus propios inputs en línea: ~50 repartidos en 23 archivos,
 * de los cuales sólo 3 usaban calendario y sólo 2 decían `required`.
 *
 * Lo que queda aquí es lo que de verdad sólo usa el catálogo: Section, el grupo
 * de radios, la alerta, la pista de vacío y el chip de selección.
 */

export interface RadioOption<Value extends string = string> {
  value: Value;
  label: string;
}

/**
 * Generic over the option value so a caller whose choices are a union (a
 * recurrence, a payment method) keeps that type end to end instead of widening
 * to `string` and casting it back.
 */
export function RadioGroup<Value extends string>({
  legend,
  options,
  value,
  onChange,
}: {
  legend: string;
  options: readonly RadioOption<Value>[];
  value: Value;
  onChange: (value: Value) => void;
}): ReactElement {
  return (
    <fieldset style={{ border: "none", padding: 0, margin: 0, minWidth: 0 }}>
      <legend style={labelStyle}>{legend}</legend>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
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
                whiteSpace: "nowrap",
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
      type="button"
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

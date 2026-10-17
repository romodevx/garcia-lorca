"use client";

import type { CSSProperties, ReactElement, ReactNode } from "react";
import { Icon, type IconName } from "../../_design/Icon";
import { HelpTip } from "../../_design/form";
import type { SchoolLevel } from "../../uniforms/api";

export type { SchoolLevel };

/**
 * Shared pieces of the uniform shop's surface.
 *
 * Everything here either reuses the design system's vocabulary or exists because
 * this module needs something the system does not have yet. Nothing invents a
 * second look.
 */

/**
 * Money crosses the wire as integer cents; pesos exist only at the boundary.
 * Re-exported rather than reimplemented — this file had its own Intl formatter
 * and the dashboard a third one that rounded to whole pesos.
 */
export { formatCents as money } from "../../_lib/money";

/** The viewport below which the panel stops being a table and becomes cards. */
export const NARROW = 768;

export const field: CSSProperties = {
  width: "100%",
  height: 48,
  padding: "0 14px",
  borderRadius: "var(--r-md)",
  border: "1.5px solid var(--border)",
  background: "var(--surface)",
  color: "var(--text)",
  fontSize: 15.5,
  outline: "none",
};

export const label: CSSProperties = {
  display: "block",
  fontSize: 13,
  fontWeight: 700,
  color: "var(--text-muted)",
  marginBottom: 6,
  fontFamily: "var(--font-display)",
};

/**
 * La etiqueta de un campo de esta pantalla, con su `(?)` al lado.
 *
 * Los campos de aquí no pasan por la capa de captura (son inputs propios con
 * `field`), así que sin esto cada uno repetiría el mismo envoltorio. El id de la
 * ayuda se deriva del del campo: quien lo use pone
 * `aria-describedby={`${id}-help`}` en su input.
 */
export function FieldLabel({
  htmlFor,
  help,
  style,
  children,
}: {
  htmlFor: string;
  help?: string;
  style?: CSSProperties;
  children: string;
}): ReactElement {
  return (
    <div style={{ display: "flex", alignItems: "baseline", gap: 6, ...style }}>
      <label htmlFor={htmlFor} style={{ ...label, marginBottom: 0 }}>
        {children}
      </label>
      {help !== undefined && <HelpTip id={`${htmlFor}-help`} label={children} text={help} />}
    </div>
  );
}

/** A field of this screen that refused the save: red border, same shape. */
export function fieldWithError(invalid: boolean): CSSProperties {
  return invalid ? { ...field, border: "1.5px solid var(--red)" } : field;
}

/**
 * What a field is missing, said under it. Its id is the field's plus
 * `-error`, so the input lists it in `aria-describedby` next to its help.
 */
export function FieldError({ htmlFor, message }: { htmlFor: string; message: string | undefined }): ReactElement | null {
  if (message === undefined) return null;
  return (
    <p id={`${htmlFor}-error`} role="alert" style={{ fontSize: 13.5, color: "var(--bad-fg)", marginTop: 6 }}>
      {message}
    </p>
  );
}

/**
 * A card that says what happened and what to do about it.
 *
 * Empty is not one state but three, and the design review was right that fusing
 * them is what produces "No items found." A shop on its first afternoon, a
 * filter that matched nothing, and a queue that has been worked to zero are
 * different news and deserve different words.
 */
export function EmptyState({
  icon,
  title,
  hint,
  action,
  tone = "neutral",
}: {
  icon: IconName;
  title: string;
  hint: string;
  action?: ReactNode;
  tone?: "neutral" | "celebrate";
}): ReactElement {
  const accent = tone === "celebrate" ? "var(--forest)" : "var(--text-faint)";
  return (
    <div
      style={{
        display: "grid",
        placeItems: "center",
        gap: 10,
        padding: "48px 24px",
        textAlign: "center",
        background: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--r-xl)",
      }}
    >
      <div
        style={{
          width: 52,
          height: 52,
          borderRadius: 16,
          display: "grid",
          placeItems: "center",
          background: `color-mix(in oklab, ${accent} 14%, var(--surface))`,
          color: accent,
        }}
      >
        <Icon name={icon} size={26} />
      </div>
      <p style={{ fontSize: 17, fontWeight: 700, color: "var(--text)", fontFamily: "var(--font-display)" }}>{title}</p>
      <p style={{ fontSize: 14.5, color: "var(--text-muted)", maxWidth: 420, lineHeight: 1.5 }}>{hint}</p>
      {action !== undefined && <div style={{ marginTop: 6 }}>{action}</div>}
    </div>
  );
}

/** A dismissible failure, in the shop's own words. */
export function ErrorNote({ message, detail, onRetry }: { message: string; detail?: string; onRetry?: () => void }): ReactElement {
  return (
    <div
      role="alert"
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: 12,
        padding: "14px 16px",
        borderRadius: "var(--r-lg)",
        background: "var(--bad-bg)",
        color: "var(--bad-fg)",
        border: "1px solid color-mix(in oklab, var(--red) 30%, transparent)",
      }}
    >
      <Icon name="warning" size={20} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontWeight: 700, fontSize: 14.5 }}>{message}</p>
        {detail !== undefined && detail !== "" && (
          <p style={{ fontSize: 13.5, opacity: 0.85, marginTop: 3 }}>{detail}</p>
        )}
      </div>
      {onRetry !== undefined && (
        <button onClick={onRetry} style={{ fontWeight: 700, fontSize: 13.5, textDecoration: "underline" }}>
          Reintentar
        </button>
      )}
    </div>
  );
}

/** A skeleton row, so a slow read looks like loading rather than emptiness. */
export function SkeletonRows({ rows = 5 }: { rows?: number }): ReactElement {
  return (
    <div style={{ display: "grid", gap: 10 }}>
      {Array.from({ length: rows }, (_, i) => (
        <div
          key={i}
          className="shimmer"
          style={{
            height: 56,
            borderRadius: "var(--r-lg)",
            background: "var(--surface-3)",
            opacity: 1 - i * 0.12,
          }}
        />
      ))}
    </div>
  );
}

/**
 * How many pieces are free, and whether that is a problem.
 *
 * It reads AVAILABLE, never what is on the shelf: five pieces all spoken for is
 * an empty shelf as far as the next parent is concerned. Colour is never the only
 * signal — the number and the word carry it for anyone who cannot see the
 * difference.
 */
export function StockPill({ available, belowMinimum }: { available: number; belowMinimum: boolean }): ReactElement {
  const empty = available <= 0;
  const tone = empty
    ? { bg: "var(--bad-bg)", fg: "var(--bad-fg)", dot: "var(--red)" }
    : belowMinimum
      ? { bg: "var(--warn-bg)", fg: "var(--warn-fg)", dot: "var(--yellow)" }
      : { bg: "var(--ok-bg)", fg: "var(--ok-fg)", dot: "var(--forest)" };
  const text = empty ? "Sin existencia" : belowMinimum ? `Quedan ${available}` : `${available} disponibles`;
  return (
    <span
      aria-label={text}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 7,
        padding: "5px 12px 5px 10px",
        borderRadius: 99,
        background: tone.bg,
        color: tone.fg,
        fontWeight: 700,
        fontSize: 13,
        fontFamily: "var(--font-display)",
        whiteSpace: "nowrap",
      }}
    >
      <span style={{ width: 9, height: 9, borderRadius: 99, background: tone.dot }} />
      {text}
    </span>
  );
}

// The numbers above the table live in the design system now; the shop
// imports them from here as it always has.
export { StatCards, type StatCardDef } from "../../_design/StatCards";

/**
 * Level chips. Tabs are for STATUS in this system; chips are for level.
 *
 * The levels are the school's own, from Configuración — never a fixed SEP list:
 * a school with Preescolar sees Preescolar. The first chip is `allLabel` (value
 * "") — "Todos" on a filter, "Todos los niveles" when scoping a garment.
 */
export function LevelChips({
  levels: schoolLevels,
  value,
  onChange,
  allLabel = "Todos",
}: {
  levels: readonly SchoolLevel[];
  value: string;
  onChange: (levelId: string) => void;
  allLabel?: string;
}): ReactElement {
  const levels = [{ id: "", label: allLabel }, ...schoolLevels.map((l) => ({ id: l.id, label: l.name }))];
  return (
    <div role="group" aria-label="Nivel" style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
      {levels.map((level) => {
        const on = level.id === value;
        return (
          <button
            key={level.id}
            aria-pressed={on}
            onClick={() => onChange(level.id)}
            style={{
              height: 36,
              padding: "0 14px",
              borderRadius: 99,
              fontSize: 13.5,
              fontWeight: 600,
              fontFamily: "var(--font-display)",
              border: on ? "1.5px solid var(--forest)" : "1px solid var(--border)",
              background: on ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
              color: on ? "var(--forest)" : "var(--text-muted)",
            }}
          >
            {level.label}
          </button>
        );
      })}
    </div>
  );
}

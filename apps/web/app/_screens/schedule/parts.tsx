"use client";

import type { CSSProperties, ReactElement, ReactNode } from "react";

import type { ClassStatus } from "../../schedule/api";

/** The pill tab group of the design system (§4). */
export function Tabs<T extends string>({ label, tabs, active, onChoose }: {
  label: string;
  tabs: readonly { id: T; label: string }[];
  active: T;
  onChoose: (id: T) => void;
}): ReactElement {
  return (
    <div role="tablist" aria-label={label} style={{ display: "flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", flexWrap: "wrap", justifySelf: "start", maxWidth: "100%" }}>
      {tabs.map((t) => {
        const on = t.id === active;
        return (
          <button key={t.id} type="button" role="tab" aria-selected={on} onClick={() => onChoose(t.id)}
            style={{ minHeight: 44, padding: "0 18px", borderRadius: 99, background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 600, fontSize: 14.5, fontFamily: "var(--font-display)", boxShadow: on ? "var(--shadow-card)" : "none" }}>
            {t.label}
          </button>
        );
      })}
    </div>
  );
}

/** What a changed class says about itself; a regular class says nothing. */
export const STATUS_LABEL: Record<ClassStatus, string> = {
  regular: "",
  substitute: "Suplencia",
  swapped: "Intercambio",
  cancelled: "Cancelada",
  uncovered: "Sin cubrir",
};

const STATUS_TONE: Record<ClassStatus, { bg: string; fg: string }> = {
  regular: { bg: "var(--ok-bg)", fg: "var(--ok-fg)" },
  substitute: { bg: "var(--warn-bg)", fg: "var(--warn-fg)" },
  swapped: { bg: "color-mix(in oklab, var(--purple) 16%, var(--surface))", fg: "var(--purple)" },
  cancelled: { bg: "var(--bad-bg)", fg: "var(--bad-fg)" },
  uncovered: { bg: "var(--bad-fg)", fg: "#fff" },
};

/** The status pill (dot + label); nothing for a class as planned. */
export function StatusBadge({ status }: { status: ClassStatus }): ReactElement | null {
  if (status === "regular") return null;
  const tone = STATUS_TONE[status];
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 6, height: 26, padding: "0 10px", borderRadius: 99, background: tone.bg, color: tone.fg, fontSize: 12.5, fontWeight: 700, whiteSpace: "nowrap" }}>
      <span aria-hidden style={{ width: 7, height: 7, borderRadius: 99, background: "currentColor" }} />
      {STATUS_LABEL[status]}
    </span>
  );
}

export const card: CSSProperties = {
  background: "var(--surface)",
  border: "1px solid var(--border)",
  borderRadius: "var(--r-lg)",
  boxShadow: "var(--shadow-card)",
};

/** A dashed box that says why there is nothing here and what to do next. */
export function EmptyState({ children }: { children: ReactNode }): ReactElement {
  return (
    <p style={{ margin: 0, padding: "18px 20px", borderRadius: "var(--r-lg)", border: "1.5px dashed var(--border-strong)", color: "var(--text-muted)", fontSize: 15, background: "var(--surface)" }}>
      {children}
    </p>
  );
}

/** A ChipRow option without a count (the shared chips want one). */
export const chipOption = (id: string, label: string): { id: string; label: string; count: number } => ({ id, label, count: 0 });

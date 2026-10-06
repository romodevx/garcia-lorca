"use client";

import type { CSSProperties, ReactElement, ReactNode } from "react";

import { Icon, type IconName } from "../../_design/Icon";
import type { RowStatus } from "../../studentImport/api";
import { STATUS_LABEL } from "../../studentImport/view";

/** The surface every block of the flow sits on. */
export const cardStyle: CSSProperties = {
  display: "grid", gap: 12, padding: "18px 20px", borderRadius: "var(--r-xl)", background: "var(--surface)",
  border: "1px solid var(--border)", boxShadow: "var(--shadow-card)", minWidth: 0,
};

export const alertStyle: CSSProperties = {
  padding: "12px 14px", borderRadius: "var(--r-md)", background: "var(--bad-bg)", color: "var(--bad-fg)",
  fontWeight: 600, fontSize: 14.5, overflowWrap: "anywhere", margin: 0,
};

export const noteStyle: CSSProperties = {
  padding: "12px 14px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)",
  color: "var(--text)", fontSize: 14.5, lineHeight: 1.5, margin: 0, overflowWrap: "anywhere",
};

export const warnStyle: CSSProperties = { ...noteStyle, background: "var(--warn-bg)", color: "var(--warn-fg)", border: "none" };

/** A choice among a few: ≥ 44 px tall, pressed state announced. */
export function Chip({ selected, onClick, children }: { selected: boolean; onClick: () => void; children: ReactNode }): ReactElement {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      style={{
        minHeight: 44, padding: "0 16px", borderRadius: "var(--r-pill)", fontSize: 14.5, fontWeight: 600,
        border: `1.5px solid ${selected ? "var(--forest)" : "var(--border)"}`,
        background: selected ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
        color: selected ? "var(--forest)" : "var(--text)",
      }}
    >
      {children}
    </button>
  );
}

const PILL: Record<RowStatus, { icon: IconName; bg: string; fg: string }> = {
  new: { icon: "check", bg: "var(--ok-bg)", fg: "var(--ok-fg)" },
  warning: { icon: "warning", bg: "var(--warn-bg)", fg: "var(--warn-fg)" },
  already: { icon: "user", bg: "var(--surface-3)", fg: "var(--text-muted)" },
  skipped: { icon: "close", bg: "var(--bad-bg)", fg: "var(--bad-fg)" },
  pending: { icon: "clock", bg: "var(--warn-bg)", fg: "var(--warn-fg)" },
};

/** The row's fate: icon AND words, never colour alone. */
export function StatusPill({ status }: { status: RowStatus }): ReactElement {
  const p = PILL[status];
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 10px", borderRadius: "var(--r-pill)", background: p.bg, color: p.fg, fontSize: 13, fontWeight: 700, whiteSpace: "nowrap" }}>
      <Icon name={p.icon} size={14} stroke={2.4} />
      {STATUS_LABEL[status]}
    </span>
  );
}

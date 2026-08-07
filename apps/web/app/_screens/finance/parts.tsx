"use client";

import type { ReactElement, ReactNode } from "react";
import { Icon, type IconName } from "../../_design/Icon";
import { COLOR, type ColorKey } from "../../_design/colors";

/**
 * Presentational primitives for the finance surface. The catalog primitives
 * (Section/TextField/SelectField/DateField/RadioGroup/…) are re-exported so a
 * panel imports everything visual from one place.
 *
 * MONEY conversion deliberately does NOT live here: it is pure logic with its
 * own unit tests, in `./money`.
 */

export {
  DateField,
  EmptyHint,
  ErrorAlert,
  RadioGroup,
  Section,
  SelectField,
  TextField,
  type SelectOption,
} from "../config/parts";

/** A single labelled checkbox (`aria-label` = the visible label, for a11y + tests). */
export function CheckBoxField({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}): ReactElement {
  return (
    <label
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 9,
        height: 44,
        padding: "0 14px",
        borderRadius: "var(--r-pill)",
        cursor: "pointer",
        border: `1.5px solid ${checked ? "var(--forest)" : "var(--border)"}`,
        background: checked ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
        color: checked ? "var(--forest)" : "var(--text-muted)",
        fontSize: 14,
        fontWeight: 600,
      }}
    >
      <input
        type="checkbox"
        aria-label={label}
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        style={{ accentColor: "var(--forest)" }}
      />
      {label}
    </label>
  );
}

// --- chrome ---------------------------------------------------------------

export interface TabDef<Id extends string> {
  id: Id;
  label: string;
}

/**
 * An accessible pill tab bar (`role="tablist"`), the surface's section switcher
 * and the charge-status filter. Labelled so tests and screen readers can tell
 * the two apart.
 */
export function TabBar<Id extends string>({
  label,
  tabs,
  active,
  onSelect,
}: {
  label: string;
  tabs: readonly TabDef<Id>[];
  active: Id;
  onSelect: (id: Id) => void;
}): ReactElement {
  return (
    <div
      role="tablist"
      aria-label={label}
      style={{
        display: "flex",
        gap: 6,
        padding: 5,
        background: "var(--surface-3)",
        borderRadius: "var(--r-pill)",
        border: "1px solid var(--border)",
        flexWrap: "wrap",
      }}
    >
      {tabs.map((tab) => {
        const on = tab.id === active;
        return (
          <button
            key={tab.id}
            role="tab"
            aria-selected={on}
            onClick={() => onSelect(tab.id)}
            style={{
              height: 40,
              padding: "0 16px",
              borderRadius: 99,
              background: on ? "var(--surface)" : "transparent",
              color: on ? "var(--text)" : "var(--text-muted)",
              fontWeight: 600,
              fontSize: 14,
              fontFamily: "var(--font-display)",
              boxShadow: on ? "var(--shadow-card)" : "none",
              transition: "all .15s var(--ease)",
            }}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}

const STATUS_TONE: Record<string, { dot: string; bg: string; fg: string }> = {
  paid: { dot: "var(--forest)", bg: "var(--ok-bg)", fg: "var(--ok-fg)" },
  partial: { dot: "var(--yellow)", bg: "var(--warn-bg)", fg: "var(--warn-fg)" },
  pending: { dot: "var(--orange)", bg: "var(--warn-bg)", fg: "var(--warn-fg)" },
  cancelled: { dot: "var(--text-faint)", bg: "var(--surface-3)", fg: "var(--text-muted)" },
};

/** Status pill for a charge; unknown statuses fall back to the neutral tone. */
export function StatusPill({ status, label }: { status: string; label: string }): ReactElement {
  const tone = STATUS_TONE[status] ?? STATUS_TONE.cancelled!;
  return (
    <span
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
      <span
        style={{
          width: 9,
          height: 9,
          borderRadius: 99,
          background: tone.dot,
          boxShadow: `0 0 0 3px color-mix(in oklab, ${tone.dot} 22%, transparent)`,
        }}
      />
      {label}
    </span>
  );
}

export interface SummaryCard {
  icon: IconName;
  color: ColorKey;
  label: string;
  value: string;
  sub: string;
}

/** The four KPI cards above the charge table. */
export function SummaryCards({ cards }: { cards: readonly SummaryCard[] }): ReactElement {
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
      {cards.map((card) => {
        const tone = COLOR[card.color];
        return (
          <div
            key={card.label}
            style={{
              flex: "1 1 200px",
              minWidth: 0,
              display: "flex",
              alignItems: "center",
              gap: 14,
              padding: "18px 20px",
              borderRadius: "var(--r-lg)",
              background: "var(--surface)",
              border: "1px solid var(--border)",
              boxShadow: "var(--shadow-card)",
            }}
          >
            <div
              style={{
                width: 46,
                height: 46,
                borderRadius: 13,
                flexShrink: 0,
                display: "grid",
                placeItems: "center",
                background: `color-mix(in oklab, ${tone.v} 16%, var(--surface))`,
                color: tone.v,
              }}
            >
              <Icon name={card.icon} size={24} />
            </div>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600, whiteSpace: "nowrap" }}>{card.label}</div>
              <div
                style={{
                  fontFamily: "var(--font-display)",
                  fontWeight: 700,
                  fontSize: 22,
                  color: "var(--text)",
                  lineHeight: 1.2,
                  whiteSpace: "nowrap",
                }}
              >
                {card.value}
              </div>
              <div style={{ fontSize: 12, color: "var(--text-faint)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                {card.sub}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

/** The friendly "nothing to collect" state for an empty charge table. */
export function AllClear({ title, hint }: { title: string; hint: string }): ReactElement {
  return (
    <div style={{ padding: "56px 20px", textAlign: "center" }}>
      <div
        style={{
          width: 70,
          height: 70,
          borderRadius: 20,
          margin: "0 auto 16px",
          display: "grid",
          placeItems: "center",
          background: "var(--ok-bg)",
          color: "var(--forest)",
        }}
      >
        <Icon name="check" size={36} stroke={2.4} />
      </div>
      <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19, color: "var(--text)" }}>{title}</div>
      <div style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 6 }}>{hint}</div>
    </div>
  );
}

/** A bottom-centred confirmation toast (`role="status"`). */
export function Toast({ children }: { children: ReactNode }): ReactElement {
  return (
    <div
      role="status"
      style={{
        position: "fixed",
        bottom: 28,
        left: "50%",
        transform: "translateX(-50%)",
        zIndex: 60,
        display: "flex",
        alignItems: "center",
        gap: 11,
        padding: "14px 22px",
        borderRadius: "var(--r-pill)",
        background: "var(--forest)",
        color: "#fff",
        fontWeight: 600,
        fontSize: 15,
        boxShadow: "var(--shadow-pop)",
        animation: "pop .25s var(--ease) both",
      }}
    >
      <Icon name="check" size={20} stroke={2.6} />
      {children}
    </div>
  );
}

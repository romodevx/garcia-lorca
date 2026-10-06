"use client";

import type { ReactElement } from "react";

import { COLOR, type ColorKey } from "./colors";
import { Icon, type IconName } from "./Icon";

export interface StatCardDef {
  icon: IconName;
  color: ColorKey;
  label: string;
  value: string;
  sub: string;
  onClick?: () => void;
  /** For tests and QA: the card's data-testid. */
  testId?: string;
}

/**
 * The numbers above the table.
 *
 * They are clickable and they apply a filter, because a number a person cannot
 * act on is decoration. `flex: 1 1 200px` is what makes them wrap on a phone
 * without a media query.
 */
export function StatCards({ cards }: { cards: readonly StatCardDef[] }): ReactElement {
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
      {cards.map((card) => {
        const tone = COLOR[card.color].v;
        const clickable = card.onClick !== undefined;
        return (
          <button
            key={card.label}
            data-testid={card.testId}
            onClick={card.onClick}
            disabled={!clickable}
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
              textAlign: "left",
              cursor: clickable ? "pointer" : "default",
            }}
          >
            <div
              style={{
                width: 44,
                height: 44,
                borderRadius: 14,
                display: "grid",
                placeItems: "center",
                background: `color-mix(in oklab, ${tone} 16%, var(--surface))`,
                color: tone,
                flexShrink: 0,
              }}
            >
              <Icon name={card.icon} size={22} />
            </div>
            <div style={{ minWidth: 0 }}>
              <p style={{ fontSize: 12.5, fontWeight: 700, color: "var(--text-muted)", textTransform: "uppercase", letterSpacing: 0.3 }}>
                {card.label}
              </p>
              <p style={{ fontSize: 22, fontWeight: 700, color: "var(--text)", fontFamily: "var(--font-display)", lineHeight: 1.2 }}>
                {card.value}
              </p>
              <p style={{ fontSize: 12.5, color: "var(--text-faint)" }}>{card.sub}</p>
            </div>
          </button>
        );
      })}
    </div>
  );
}

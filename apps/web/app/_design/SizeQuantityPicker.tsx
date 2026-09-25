"use client";

import { useRef, useState, type ReactElement } from "react";

import { formatCents } from "../_lib/money";
import { Btn } from "./Controls";

/** One size of a garment as the shelf has it. */
export interface PickableSize {
  label: string;
  /** Free to sell now: on hand minus what families have on hold. */
  available: number;
}

/** A size and how many of it go into the cart. */
export interface SizePick {
  sizeLabel: string;
  qty: number;
}

export interface SizeQuantityPickerProps {
  /** In the order the shelf shows them. */
  sizes: readonly PickableSize[];
  unitPriceCents: number;
  /** Units of each size already in the cart: they are not free any more. */
  inCart?: Readonly<Record<string, number>>;
  /** Every size with a quantity, in one call. */
  onAdd: (picks: SizePick[]) => void;
}

const NOTHING_CHOSEN = "Elige al menos una talla.";
const RAN_OUT = "Ya no queda existencia libre de lo que elegiste: elige otra talla.";

/**
 * Sizes and quantities in one pass (task 11). A tap on a size puts one in; its
 * row below takes it up or down, never past what is free; one button adds every
 * size chosen. The chips only choose — at 360 px a chip is ~72 px wide, too
 * narrow for two 44 px targets — so each chosen size gets a full-width row
 * with a stepper a thumb can hit.
 */
export function SizeQuantityPicker({ sizes, unitPriceCents, inCart = {}, onAdd }: SizeQuantityPickerProps): ReactElement {
  const [qty, setQty] = useState<Record<string, number>>({});
  const [error, setError] = useState("");
  // The chips, by size: focus comes back to one when its row goes away, and to
  // the first on sale when "Agregar" finds nothing chosen.
  const chips = useRef(new Map<string, HTMLButtonElement>());

  const free = (size: PickableSize): number => Math.max(0, size.available - (inCart[size.label] ?? 0));
  const set = (label: string, next: number): void => {
    setError("");
    setQty((current) => {
      const copy = { ...current };
      if (next <= 0) {
        delete copy[label];
      } else {
        copy[label] = next;
      }
      return copy;
    });
  };

  const chosen = sizes.filter((s) => (qty[s.label] ?? 0) > 0);
  const pieces = chosen.reduce((sum, s) => sum + (qty[s.label] ?? 0), 0);

  const drop = (label: string): void => {
    set(label, 0);
    chips.current.get(label)?.focus();
  };

  const add = (): void => {
    // Capped at what is free NOW: the shelf may have shrunk since the choice
    // (a reload after an out-of-stock sale), and the server would refuse more.
    const picks = chosen
      .map((s) => ({ sizeLabel: s.label, qty: Math.min(qty[s.label] ?? 0, free(s)) }))
      .filter((p) => p.qty > 0);
    if (picks.length === 0) {
      // Chosen but gone from the shelf is not the same as nothing chosen: say
      // which, and clear the stale choices.
      setQty({});
      setError(chosen.length > 0 ? RAN_OUT : NOTHING_CHOSEN);
      const first = sizes.find((s) => free(s) > 0);
      if (first !== undefined) {
        chips.current.get(first.label)?.focus();
      }
      return;
    }
    onAdd(picks);
    setQty({});
  };

  return (
    <div style={{ display: "grid", gap: 12 }}>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(72px, 1fr))", gap: 8 }}>
        {sizes.map((size) => {
          const soldOut = free(size) <= 0;
          const picked = qty[size.label] ?? 0;
          return (
            <button
              key={size.label}
              type="button"
              aria-disabled={soldOut}
              aria-pressed={picked > 0}
              aria-label={chipLabel(size.label, picked, free(size))}
              ref={(el) => {
                if (el === null) {
                  chips.current.delete(size.label);
                } else {
                  chips.current.set(size.label, el);
                }
              }}
              onClick={() => {
                if (soldOut) {
                  return;
                }
                // A toggle, as aria-pressed says: a second tap takes it out.
                set(size.label, picked > 0 ? 0 : 1);
              }}
              style={{
                minHeight: 64,
                padding: "6px 4px",
                borderRadius: "var(--r-md)",
                border: picked > 0 ? "1.5px solid var(--forest)" : "1px solid var(--border)",
                background: picked > 0 ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
                opacity: soldOut ? 0.45 : 1,
                cursor: soldOut ? "not-allowed" : "pointer",
                display: "grid",
                placeItems: "center",
                gap: 2,
              }}
            >
              <span style={{ fontWeight: 700, fontSize: 16, color: "var(--text)" }}>{size.label}</span>
              <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
                {soldOut ? "agotada" : picked > 0 ? `× ${picked}` : `${free(size)} libres`}
              </span>
            </button>
          );
        })}
      </div>

      {chosen.map((size) => (
        <div key={size.label} style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <span style={{ flex: 1, minWidth: 0, fontSize: 15, color: "var(--text)" }}>Talla {size.label}</span>
          <Stepper
            label={size.label}
            qty={qty[size.label] ?? 0}
            max={free(size)}
            onChange={(next) => (next <= 0 ? drop(size.label) : set(size.label, next))}
          />
        </div>
      ))}

      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <span data-testid="size-pick-summary" style={{ flex: 1, minWidth: 0, fontSize: 14.5, color: "var(--text-muted)" }}>
          {pieces === 1 ? "1 pieza" : `${pieces} piezas`} · {formatCents(pieces * unitPriceCents)}
        </span>
        <Btn kind="primary" onClick={add}>
          Agregar al carrito
        </Btn>
      </div>
      {error !== "" && (
        <p role="alert" style={{ fontSize: 13.5, color: "var(--bad-fg)" }}>
          {error}
        </p>
      )}
    </div>
  );
}

/** What a screen reader hears for a chip: the size, what is chosen and what is free. */
function chipLabel(label: string, picked: number, free: number): string {
  if (free <= 0 && picked === 0) {
    return `Talla ${label}, agotada`;
  }
  if (picked > 0) {
    return `Talla ${label}, ${picked} ${picked === 1 ? "elegida" : "elegidas"} de ${free} libres`;
  }
  return `Talla ${label}, ${free} libres`;
}

function Stepper({ label, qty, max, onChange }: { label: string; qty: number; max: number; onChange: (next: number) => void }): ReactElement {
  const atMax = qty >= max;
  const square = {
    width: 44,
    height: 44,
    borderRadius: "var(--r-md)",
    border: "1px solid var(--border)",
    background: "var(--surface)",
    fontSize: 20,
    fontWeight: 700,
    color: "var(--text)",
  } as const;
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
      <button type="button" aria-label={`Una menos de talla ${label}`} onClick={() => onChange(qty - 1)} style={square}>
        −
      </button>
      <output aria-label={`Cantidad talla ${label}`} style={{ minWidth: 32, textAlign: "center", fontWeight: 700, fontSize: 16 }}>
        {qty}
      </output>
      <button
        type="button"
        aria-label={`Una más de talla ${label}`}
        aria-disabled={atMax}
        onClick={() => {
          if (!atMax) {
            onChange(qty + 1);
          }
        }}
        style={{ ...square, opacity: atMax ? 0.45 : 1, cursor: atMax ? "not-allowed" : "pointer" }}
      >
        +
      </button>
    </div>
  );
}

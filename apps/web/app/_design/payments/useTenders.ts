"use client";

import { useCallback, useMemo, useRef, useState } from "react";

import type { CardType, PaymentMethod, Tender } from "../../finance/api";
import { formatPesosInput, pesosToCents } from "../../_lib/money";
import {
  MAX_TENDERS,
  amountError,
  coveredCents,
  nextMethod,
  tenderErrors,
  toTenders,
  type TenderRow,
} from "./tenders";

/** What a screen holds to take a payment split across methods (T8). */
export interface TendersState {
  /** The amount the rows must add up to; NaN while the screen's own amount is unreadable. */
  totalCents: number;
  rows: readonly TenderRow[];
  errors: Readonly<Record<string, string>>;
  /** Anything the cashier changed: a second row, another method, a reference, an amount of their own. */
  dirty: boolean;
  setMethod: (key: string, method: PaymentMethod) => void;
  setCardType: (key: string, cardType: CardType) => void;
  setAmount: (key: string, amount: string) => void;
  setReference: (key: string, reference: string) => void;
  blurAmount: (key: string) => void;
  add: () => void;
  remove: (key: string) => void;
  /** Marks what blocks the submit; true when nothing does. Focusing it is the screen's job. */
  validate: () => boolean;
  toTenders: () => Tender[];
  reset: () => void;
}

/** The amount a following row shows: the total, once it is readable money. */
function followed(totalCents: number): string {
  return Number.isFinite(totalCents) && totalCents > 0 ? formatPesosInput(totalCents) : "";
}

/**
 * The rows of a split payment and the rules that go with them.
 *
 * The 80% case is one cash row for the whole amount, and it must stay one
 * click: while there is a single row whose amount nobody typed, that amount
 * FOLLOWS the total — the cashier changes "Monto a pagar" and the row keeps up.
 * Typing another amount, or adding a row, stops it.
 */
export function useTenders(totalCents: number, initial?: readonly Tender[]): TendersState {
  const nextKey = useRef(Math.max(1, initial?.length ?? 0));
  const mint = useCallback((): string => `t${nextKey.current++}`, []);
  // A payment being corrected (T9) opens with the methods it was recorded with.
  const [stored, setStored] = useState<TenderRow[]>(() => (initial !== undefined && initial.length > 0 ? initial.map(fromTender) : [blank("t0", "cash")]));
  // One row keeps following the amount, whatever its method; several were typed.
  const [following, setFollowing] = useState(() => initial === undefined || initial.length <= 1);
  const [errors, setErrors] = useState<Record<string, string>>({});

  // The rows as they are now, with a following row carrying the total.
  const rows = useMemo<TenderRow[]>(() => {
    const [first] = stored;
    if (!following || stored.length !== 1 || first === undefined) return stored;
    return [{ ...first, amount: followed(totalCents) }];
  }, [stored, following, totalCents]);

  // An error stays only while it is still the same error: typing it right
  // clears it, and a different mistake waits for the blur or the submit — an
  // empty amount being filled is not yet a sum that does not add up.
  const commit = useCallback((next: TenderRow[]): void => {
    setStored(next);
    setErrors((current) => {
      const fresh = tenderErrors(next, totalCents);
      const kept: Record<string, string> = {};
      for (const [key, message] of Object.entries(current)) {
        if (fresh[key] === message) kept[key] = message;
      }
      return kept;
    });
  }, [totalCents]);

  const patch = useCallback((key: string, change: Partial<TenderRow>): void => {
    commit(rows.map((r) => (r.key === key ? { ...r, ...change } : r)));
  }, [commit, rows]);

  const setAmount = useCallback((key: string, amount: string): void => {
    // Reformatting the followed amount on blur is not choosing another one.
    if (pesosToCents(amount) !== totalCents) setFollowing(false);
    patch(key, { amount });
  }, [patch, totalCents]);

  const blurAmount = useCallback((key: string): void => {
    const row = rows.find((r) => r.key === key);
    const wrong = row === undefined ? undefined : amountError(row);
    setErrors((current) => {
      const next = { ...current };
      if (wrong === undefined) delete next[key];
      else next[key] = wrong;
      return next;
    });
  }, [rows]);

  const add = useCallback((): void => {
    if (rows.length >= MAX_TENDERS) return;
    // The new row offers what is left, so completing a credit note is one click.
    const left = (Number.isFinite(totalCents) ? totalCents : 0) - coveredCents(rows);
    setFollowing(false);
    commit([...rows, { ...blank(mint(), nextMethod(rows)), amount: left > 0 ? formatPesosInput(left) : "" }]);
  }, [commit, mint, rows, totalCents]);

  const remove = useCallback((key: string): void => {
    if (rows.length <= 1) return;
    commit(rows.filter((r) => r.key !== key));
  }, [commit, rows]);

  const validate = useCallback((): boolean => {
    const found = tenderErrors(rows, totalCents);
    setErrors(found);
    return Object.keys(found).length === 0;
  }, [rows, totalCents]);

  const reset = useCallback((): void => {
    setStored([blank(mint(), "cash")]);
    setFollowing(true);
    setErrors({});
  }, [mint]);

  return {
    totalCents,
    rows,
    errors,
    dirty: !following || rows.length > 1 || rows.some((r) => r.method !== "cash" || r.reference.trim() !== ""),
    setMethod: (key, method) => patch(key, { method }),
    setCardType: (key, cardType) => patch(key, { cardType }),
    setAmount,
    setReference: (key, reference) => patch(key, { reference }),
    blurAmount,
    add,
    remove,
    validate,
    toTenders: () => toTenders(rows),
    reset,
  };
}

// A recorded card without a type (before CC-205) opens with none chosen: the
// cashier says which, rather than the screen guessing debit (N6).
function fromTender(t: Tender, i: number): TenderRow {
  return { key: `t${i}`, method: t.method, cardType: t.cardType ?? "", amount: formatPesosInput(t.amountCents), reference: t.reference ?? "" };
}

function blank(key: string, method: PaymentMethod): TenderRow {
  return { key, method, cardType: "debit", amount: "", reference: "" };
}

"use client";

import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { DateField, MoneyField, useFocusFirstInvalid } from "../../_design/form";
import { TenderEditor } from "../../_design/payments/TenderEditor";
import { AMOUNT_ERROR } from "../../_design/payments/tenders";
import { depositRequest, newAttemptKey } from "../../_design/payments/depositRequest";
import { useTenders } from "../../_design/payments/useTenders";
import { ClosedCutNotice } from "../../_design/payments/ClosedCutNotice";
import { useClosedCut } from "../../_design/payments/useClosedCut";
import {
  getAccount,
  getExpected,
  previewDeposit,
  registerDeposit,
  type Charge,
  type DepositResult,
  type Expected,
  type ExpectedLine,
  type Payment,
} from "../../finance/api";
import { groupByCharge } from "../../finance/allocations";
import { API_URL } from "../../api-url";
import { localISODate, shortDayLabel } from "../../_lib/localDate";
import { DebtList } from "./DebtList";
import { allocationLabel, DepositReceipt } from "./DepositReceipt";
import { formatPesosInput, formatCents, pesosToCents } from "../../_lib/money";
import { formatStudentName } from "../../_design/students/studentName";
import { StudentNotesBeside, useNotesBeside, type NotesAccess } from "../../_design/students/StudentNotesBeside";

/**
 * What is missing, or over, against what should have been paid. Zero when the
 * family paid exactly right — which is the number the counter looks for.
 *
 * An unreadable amount counts as nothing captured, so the difference is the
 * whole expected figure rather than NaN.
 */
function difference(captured: number, expectedCents: number): number {
  return expectedCents - (Number.isFinite(captured) ? captured : 0);
}

/** How long the typing has to stop before we ask the server what it covers. */
const PREVIEW_DEBOUNCE_MS = 350;

const lineRow: CSSProperties = { display: "flex", justifyContent: "space-between", padding: "12px 16px", borderTop: "1px solid var(--border)" };

/** The pupil the counter is collecting for. Only what the header needs. */
export interface CollectStudent {
  id: string;
  firstName: string;
  lastName: string;
  maternalLastName?: string;
  gradeGroup?: string;
}

export interface CollectDrawerProps {
  student: CollectStudent;
  onClose: () => void;
  onCollected: () => void;
  /** The office's day (aaaa-mm-dd); injected by tests. Defaults to today, locally. */
  today?: string;
  /**
   * finance:write: may grant the early-payment reward after its window. It is a
   * discount, so the Cobranza role is not offered it (task 9); the gateway
   * refuses it anyway.
   */
  canGrantDiscounts?: boolean;
  /**
   * What the cashier may do with the pupil's notes (T6). Without it, or
   * without students:read, the counter shows no notes.
   */
  notes?: NotesAccess;
}

const FUTURE_PAYMENT = "La fecha de pago no puede ser futura.";
const MISSING_PAYMENT_DATE = "Falta la fecha de pago.";

/** The concepts a quote asks for: those due that day. */
function dueOf(lines: readonly ExpectedLine[]): ReadonlySet<string> {
  return new Set(lines.filter((l) => l.due).map((l) => l.chargeId));
}

/** What the chosen concepts add up to — the one sum the list, the amount and the difference share. */
export function selectedTotal(lines: readonly ExpectedLine[], chosen: ReadonlySet<string>): number {
  return lines.reduce((sum, l) => sum + (chosen.has(l.chargeId) ? l.totalCents : 0), 0);
}

/** The ids to send, oldest first, or undefined when the choice is what is due. */
function choiceOf(lines: readonly ExpectedLine[], chosen: ReadonlySet<string>): string[] | undefined {
  const due = dueOf(lines);
  const same = due.size === chosen.size && [...chosen].every((id) => due.has(id));
  return same ? undefined : lines.filter((l) => chosen.has(l.chargeId)).map((l) => l.chargeId);
}

/**
 * The most the counter takes (T8 · T5): every line the quote lists at its
 * figure — the ones not yet open included, which may be paid ahead — or what
 * is asked today, or what was chosen, whichever is largest.
 */
export function maxPayableCents(expected: Pick<Expected, "amountCents" | "lines">, selectedCents: number): number {
  const listed = expected.lines.reduce((sum, l) => sum + l.totalCents, 0);
  return Math.max(listed, expected.amountCents, selectedCents);
}

/**
 * What is wrong with the amount to pay, if anything. It may not exceed what the
 * family owes (T8) — `capCents`, unknown (Infinity) until the quote is read.
 */
export function amountToPayError(cents: number, capCents: number): string | undefined {
  if (!Number.isFinite(cents) || cents <= 0) return AMOUNT_ERROR;
  return cents > capCents ? `No puede ser mayor que el adeudo (${formatCents(capCents)}).` : undefined;
}

/** What is wrong with a payment date, if anything. */
export function paymentDateError(paidOn: string, day: string): string | undefined {
  if (paidOn === "") return MISSING_PAYMENT_DATE;
  return paidOn > day ? FUTURE_PAYMENT : undefined;
}

/**
 * The counter, as the school actually works it: ONE number (what the family
 * owes) and ONE field (what they pay now) — paid in cash unless the cashier
 * says otherwise, a credit note and a split included (T8).
 *
 * The cashier is not asked which months the money covers. That question is the
 * reason the old screen went unused — it made a person do, per parent, the
 * arithmetic the system already knows how to do. Here the split is previewed
 * while they type and taken when they press Enter.
 */
export function CollectDrawer({ student, onClose, onCollected, today, canGrantDiscounts = false, notes }: CollectDrawerProps): ReactElement {
  // The pupil's notes on the left of the counter (T6): what the office must
  // remember about this family while it takes their money.
  const beside = useNotesBeside(student.id, notes);
  const day = today ?? localISODate();
  // The day the family paid, which is not always the day it is keyed: a
  // cashier catching up on Monday records Friday. Every rule reads it — the
  // surcharge, the early-payment window, the inscription's price (task 7).
  const [paidOn, setPaidOn] = useState(day);
  const [dateError, setDateError] = useState<string | undefined>(undefined);
  const dateOk = paymentDateError(paidOn, day) === undefined;
  // Today is not sent: the server stamps its own clock and keeps the hour of
  // the payment. A date alone reaches it as midnight UTC — 18:00 the day before
  // in Mexico City — so only a payment dated earlier travels with its date.
  const sentDate = paidOn === day ? "" : paidOn;
  const focusFirstInvalid = useFocusFirstInvalid();
  // Said before the cashier confirms, not discovered later on the cut (ISSUE-026).
  const cutClosed = useClosedCut(sentDate, dateOk);
  // Once the cashier types another amount, a new quote (a new date) does not
  // overwrite it. Reformatting the prefilled amount on blur is not that.
  const amountEdited = useRef(false);
  const prefilledCents = useRef<number | undefined>(undefined);
  // The day the quote ON SCREEN is for ("" = today). The label follows the
  // quote, not the field: a date that brought no quote does not relabel it.
  const [expectedFor, setExpectedFor] = useState("");
  // The concepts the money pays (task 8). Each new quote starts with what is
  // due — what the counter asks for today, the rule as it always was — and a
  // choice is sent only when it differs from that. A month not yet due can be
  // chosen: that is paying ahead.
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set());
  const [owed, setOwed] = useState<number | null>(null);
  const [credit, setCredit] = useState(0);
  const [expected, setExpected] = useState<Expected | null>(null);
  const [amount, setAmount] = useState("");
  // The school honouring the early-payment reward after the window closed. A
  // decision somebody makes, which is why it is a switch and not a default.
  const [grantEarly, setGrantEarly] = useState(false);
  const [amountError, setAmountError] = useState<string | undefined>(undefined);
  const [preview, setPreview] = useState<{ allocations: Payment[]; surcharges: Charge[]; unallocatedCents: number } | null>(null);
  const [receipt, setReceipt] = useState<DepositResult | null>(null);
  const [saving, setSaving] = useState(false);
  // A ref and not the state: two presses inside one render both read `saving` as false.
  const inFlight = useRef(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [attemptKey] = useState(newAttemptKey);
  const describe = useRef<Map<string, string>>(new Map());

  const name = formatStudentName(student);
  const cents = pesosToCents(amount);
  const lines = expected?.lines ?? [];
  const choice = choiceOf(lines, chosen);
  const choiceKey = choice?.join(",") ?? "";
  const selectedCents = selectedTotal(lines, chosen);
  // The difference is measured against what was chosen — which, by default, is
  // exactly what is due.
  const target = lines.length > 0 ? selectedCents : (expected?.amountCents ?? 0);
  // What the family owes: the year's balance, or what is asked today when the
  // quote carries a surcharge the balance does not have yet.
  // ONE balance (T5): the quote's, for the day asked about, once it is here —
  // the figure the debt list totals too. Until then, the account's.
  const balance = expected?.annualBalanceCents ?? owed;
  // The cap is the GROSS debt the quote lists — every line, those not yet open
  // included — never the net balance: that one leaves them out and nets the
  // credit (it can be below zero), and would refuse a legitimate payment ahead.
  // Unknown (Infinity) until the quote is here; the server has the last word.
  const capCents = expected === null ? Number.POSITIVE_INFINITY : maxPayableCents(expected, selectedCents);
  // How the amount is paid, method by method (T8): it must add up to it.
  const tenders = useTenders(cents);
  const toggle = (chargeId: string): void => {
    const next = new Set(chosen);
    if (next.has(chargeId)) next.delete(chargeId);
    else next.add(chargeId);
    setChosen(next);
    // The amount follows the choice until the cashier types another one.
    if (!amountEdited.current) {
      const cents = selectedTotal(lines, next);
      prefilledCents.current = cents;
      setAmount(formatPesosInput(cents));
    }
  };

  useEffect(() => {
    let live = true;
    getAccount(API_URL, student.id)
      .then((account) => {
        if (!live) {
          return;
        }
        setOwed(account.totalBalanceCents);
        setCredit(account.unallocatedCents);
        // The split comes back as charge ids; a person reads month names.
        describe.current = new Map(account.charges.map((c) => [c.id, c.description]));
      })
      .catch(() => { if (live) { setOwed(0); } });
    return () => { live = false; };
  }, [student.id]);

  // What is due on the day the family paid, which is not the same question as
  // what is left for the year. The cashier reads this figure aloud.
  useEffect(() => {
    if (!dateOk) {
      return;
    }
    let live = true;
    const askedFor = sentDate;
    getExpected(API_URL, student.id, askedFor)
      .then((e) => {
        if (!live) {
          return;
        }
        setExpected(e);
        setExpectedFor(askedFor);
        // A new quote is a new list: the choice starts over at what is due.
        setChosen(dueOf(e.lines));
        // Pre-filled with what should be paid: the common case is a family
        // paying exactly that, and retyping it is a chance to mistype it.
        if (e.amountCents > 0 && !amountEdited.current) {
          prefilledCents.current = e.amountCents;
          setAmount(formatPesosInput(e.amountCents));
        }
      })
      .catch(() => {});
    return () => { live = false; };
  }, [student.id, sentDate, dateOk]);

  // Ask the server what the typed amount covers, once the typing settles. The
  // rule lives in one place — the same planner that takes the money — so the
  // preview can never drift from what actually happens.
  useEffect(() => {
    if (!Number.isFinite(cents) || cents <= 0 || receipt !== null || !dateOk) {
      setPreview(null);
      return;
    }
    let live = true;
    const timer = setTimeout(() => {
      previewDeposit(API_URL, {
        studentId: student.id,
        amountCents: cents,
        ...(sentDate !== "" ? { paidAt: sentDate } : {}),
        ...(choiceKey !== "" ? { chargeIds: choiceKey.split(",") } : {}),
      })
        .then((p) => { if (live) { setPreview(p); } })
        .catch(() => { if (live) { setPreview(null); } });
    }, PREVIEW_DEBOUNCE_MS);
    return () => { live = false; clearTimeout(timer); };
  }, [cents, student.id, receipt, sentDate, dateOk, choiceKey]);

  // Escape closes. Nothing is at risk: the money has not been taken, and the
  // amount is one field to retype.
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => { if (e.key === "Escape") { onClose(); } };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const submit = useCallback(async (): Promise<void> => {
    if (inFlight.current) {
      return;
    }
    // Every problem is marked at once, then the view goes to the first of them.
    const wrongDate = paymentDateError(paidOn, day);
    const wrongAmount = amountToPayError(cents, capCents);
    setDateError(wrongDate);
    setAmountError(wrongAmount);
    const tendersOk = wrongAmount === undefined && tenders.validate();
    if (wrongDate !== undefined || !tendersOk) {
      focusFirstInvalid();
      return;
    }
    inFlight.current = true;
    setSaving(true);
    setError(undefined);
    try {
      const result = await registerDeposit(API_URL, depositRequest({
        studentId: student.id,
        amountCents: cents,
        tenders: tenders.toTenders(),
        attemptKey,
        paidOn: sentDate,
        ...(choice !== undefined ? { chargeIds: choice } : {}),
        grantEarlyPayment: grantEarly,
      }));
      setReceipt(result);
      onCollected();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo registrar el cobro.");
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
  }, [student.id, cents, capCents, tenders, attemptKey, grantEarly, sentDate, paidOn, day, choice, focusFirstInvalid, onCollected]);

  return (
    <DrawerShell
      title={`Cobrar · ${name}`}
      subtitle={student.gradeGroup ?? ""}
      icon="card"
      size={receipt === null && beside.wide ? "xl" : "lg"}
      onClose={onClose}
      // This drawer's backdrop never closed it, on purpose: it is the counter,
      // and an amount, payment methods and references already typed must not be
      // lost to a stray click. The shared shell closes by default, so it has to
      // be said here. The ✕ and Cancelar still close it.
      dismissible={false}
      padded={false}
    >
      <>
        {receipt !== null
          ? <DepositReceipt result={receipt} describe={describe.current} onClose={onClose} />
          : (
            <StudentNotesBeside beside={beside} studentId={student.id} bare>
              <form
                onSubmit={(e) => { e.preventDefault(); void submit(); }}
                style={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
                <div style={{ flex: 1, overflowY: "auto", padding: "22px 26px", display: "flex", flexDirection: "column", gap: 20 }}>
                  <div style={{ padding: "18px 20px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
                    <span style={{ fontSize: 14, color: "var(--text-muted)" }}>{expectedFor === "" ? "Monto esperado hoy" : `Monto esperado al ${shortDayLabel(expectedFor)}`}</span>
                    <div data-testid="amount-expected" style={{ fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 38, color: "var(--text)", marginTop: 4 }}>
                      {expected === null ? "…" : formatCents(expected.amountCents)}
                    </div>
                    <p style={{ fontSize: 13, color: "var(--text-faint)", marginTop: 6 }}>
                      {expected !== null && expected.surchargeCents > 0 && `Incluye ${formatCents(expected.surchargeCents)} de recargo. `}
                      {expected !== null && expected.discountCents > 0 && `Ya con ${formatCents(expected.discountCents)} de pronto pago. `}
                      <YearBalance balanceCents={balance} surchargeCents={expected?.surchargeCents ?? 0} creditCents={credit} />
                    </p>
                  </div>

                  <DebtList lines={lines} balanceCents={balance ?? 0} onTimeCents={expected?.onTimeBalanceCents ?? balance ?? 0} chosen={chosen} selectedCents={selectedCents} onToggle={toggle} />

                  <MoneyField
                    label="Monto a pagar"
                    value={amount}
                    onChange={(v) => {
                      if (pesosToCents(v) !== prefilledCents.current) amountEdited.current = true;
                      setAmount(v);
                      if (amountToPayError(pesosToCents(v), capCents) === undefined) setAmountError(undefined);
                    }}
                    onBlur={() => setAmountError(amountToPayError(cents, capCents))}
                    error={amountError}
                    help="Lo que la familia paga ahora: puede ser un abono, nunca más de lo que debe."
                    required
                  />

                  {expected !== null && (
                    <div
                      data-testid="difference"
                      style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "12px 16px", borderRadius: "var(--r-md)", background: difference(cents, target) === 0 ? "var(--ok-bg)" : "var(--warn-bg)" }}>
                      <span style={{ fontSize: 14, color: "var(--text-muted)" }}>Diferencia</span>
                      <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 17, color: difference(cents, target) === 0 ? "var(--ok-fg)" : "var(--warn-fg)" }}>
                        {formatCents(difference(cents, target))}
                      </span>
                    </div>
                  )}

                  {canGrantDiscounts && expected !== null && expected.discountCents === 0 && (
                    <label style={{ display: "flex", alignItems: "center", gap: 10, cursor: "pointer" }}>
                      <input type="checkbox" checked={grantEarly} onChange={(e) => setGrantEarly(e.target.checked)} aria-label="Aplicar pronto pago" style={{ width: 18, height: 18 }} />
                      <span style={{ fontSize: 14, color: "var(--text-muted)" }}>
                        Aplicar pronto pago de todos modos
                      </span>
                    </label>
                  )}

                  {preview !== null && (
                    <div data-testid="preview" style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden" }}>
                      <div style={{ padding: "10px 16px", background: "var(--surface-2)", fontSize: 13, fontWeight: 700, color: "var(--text-muted)", fontFamily: "var(--font-display)" }}>
                        Cubre
                      </div>
                      {groupByCharge(preview.allocations).map((a) => (
                        <div key={a.chargeId} style={lineRow}>
                          <span style={{ fontSize: 14.5, color: "var(--text)" }}>{allocationLabel(a.chargeId, preview.surcharges, describe.current)}</span>
                          <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15, color: "var(--text)" }}>{formatCents(a.amountCents)}</span>
                        </div>
                      ))}
                      {preview.allocations.length === 0 && (
                        <p style={{ padding: "12px 16px", margin: 0, fontSize: 14, color: "var(--text-muted)" }}>
                          No hay nada vencido; todo queda a favor.
                        </p>
                      )}
                      {preview.unallocatedCents > 0 && (
                        <div style={{ display: "flex", justifyContent: "space-between", padding: "12px 16px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>
                          <span style={{ fontSize: 14.5, color: "var(--text-muted)" }}>Queda a favor</span>
                          <span data-testid="credit" style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15, color: "var(--forest)" }}>
                            {formatCents(preview.unallocatedCents)}
                          </span>
                        </div>
                      )}
                    </div>
                  )}

                  <TenderEditor tenders={tenders} />

                  <DateField
                    label="Fecha de pago"
                    value={paidOn}
                    max={day}
                    onChange={(v) => {
                      setPaidOn(v);
                      if (paymentDateError(v, day) === undefined) setDateError(undefined);
                    }}
                    onBlur={() => setDateError(paymentDateError(paidOn, day))}
                    error={dateError}
                    help="El día en que la familia pagó. Si capturas tarde, pon ese día: los recargos, el pronto pago y el precio de la inscripción se calculan con él."
                  />

                  <ClosedCutNotice closed={cutClosed} day={sentDate} />

                  {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>}
                </div>

                <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "18px 26px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>
                  <Btn kind="ghost" type="button" onClick={onClose}>Cancelar</Btn>
                  <div style={{ flex: 1 }} />
                  <Btn icon="check" type="submit" busy={saving} style={{ height: 52, fontSize: 16 }}>{saving ? "Cobrando…" : "Cobrar"}</Btn>
                </div>
              </form>
            </StudentNotesBeside>
          )}
      </>
    </DrawerShell>
  );
}

interface YearBalanceProps {
  /** The one balance; null while unknown. Below zero is the family's credit. */
  balanceCents: number | null;
  /** The surcharge accrued to the day, already inside the balance. */
  surchargeCents: number;
  /** Money no charge claimed, already netted out of the balance. */
  creditCents: number;
}

/** "Del año debe" — the single balance, said so the cashier reads it once (T5). */
function YearBalance({ balanceCents, surchargeCents, creditCents }: YearBalanceProps): ReactElement {
  if (balanceCents === null) {
    return <>Del año debe <b data-testid="amount-owed">…</b>.</>;
  }
  if (balanceCents < 0) {
    return <>Tiene saldo a favor <b data-testid="amount-owed">{formatCents(-balanceCents)}</b>.</>;
  }
  return (
    <>
      Del año debe <b data-testid="amount-owed">{formatCents(balanceCents)}</b>
      {surchargeCents > 0 && ", con el recargo incluido"}
      {creditCents > 0 && `, ya descontados ${formatCents(creditCents)} a favor`}.
    </>
  );
}

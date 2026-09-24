"use client";

/** Lo que el (?) de Referencia explica, en los dos lugares donde se cobra. */
const REFERENCIA_AYUDA =
  "El folio del voucher o el número de la transferencia: lo que te permita reconocer este pago después. En efectivo puede quedar vacío.";

import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { DateField, HelpTip, MoneyField, useFocusFirstInvalid } from "../../_design/form";
import {
  getAccount,
  getExpected,
  previewDeposit,
  registerDeposit,
  type Charge,
  type DepositResult,
  type Expected,
  type CardType,
  type Payment,
  type PaymentMethod,
} from "../../finance/api";
import { API_URL } from "../../api-url";
import { localISODate, shortDayLabel } from "../../_lib/localDate";
import { DebtList } from "./DebtList";
import { centsToPesosInput, formatCents, pesosToCents } from "../../_lib/money";

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

/** One key per attempt, kept across retries: a double click must not cost twice. */
function newAttemptKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/** How long the typing has to stop before we ask the server what it covers. */
const PREVIEW_DEBOUNCE_MS = 350;

const METHODS: { value: PaymentMethod; label: string }[] = [
  { value: "cash", label: "Efectivo" },
  { value: "card", label: "Tarjeta" },
  { value: "transfer", label: "Transferencia" },
];

const field: CSSProperties = {
  width: "100%", height: 64, padding: "0 18px", borderRadius: "var(--r-md)",
  border: "1.5px solid var(--border)", background: "var(--surface)", color: "var(--text)",
  fontSize: 30, fontFamily: "var(--font-display)", fontWeight: 700, outline: "none",
};

/** The pupil the counter is collecting for. Only what the header needs. */
export interface CollectStudent {
  id: string;
  firstName: string;
  lastName: string;
  gradeGroup?: string;
}

export interface CollectDrawerProps {
  student: CollectStudent;
  onClose: () => void;
  onCollected: () => void;
  /** The office's day (aaaa-mm-dd); injected by tests. Defaults to today, locally. */
  today?: string;
}

const FUTURE_PAYMENT = "La fecha de pago no puede ser futura.";
const MISSING_PAYMENT_DATE = "Falta la fecha de pago.";

/** What is wrong with a payment date, if anything. */
export function paymentDateError(paidOn: string, day: string): string | undefined {
  if (paidOn === "") return MISSING_PAYMENT_DATE;
  return paidOn > day ? FUTURE_PAYMENT : undefined;
}

/**
 * The counter, as the school actually works it: ONE number (what the family
 * owes) and ONE field (what they handed over).
 *
 * The cashier is not asked which months the money covers. That question is the
 * reason the old screen went unused — it made a person do, per parent, the
 * arithmetic the system already knows how to do. Here the split is previewed
 * while they type and taken when they press Enter.
 */
export function CollectDrawer({ student, onClose, onCollected, today }: CollectDrawerProps): ReactElement {
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
  // Once the cashier types another amount, a new quote (a new date) does not
  // overwrite it. Reformatting the prefilled amount on blur is not that.
  const amountEdited = useRef(false);
  const prefilledCents = useRef<number | undefined>(undefined);
  // The day the quote ON SCREEN is for ("" = today). The label follows the
  // quote, not the field: a date that brought no quote does not relabel it.
  const [expectedFor, setExpectedFor] = useState("");
  // Concepts the cashier took off the debt list (task 8). Everything starts
  // chosen — the rule as it always was, the oldest debt first — and a choice
  // is sent only when something was left out.
  const [unchosen, setUnchosen] = useState<ReadonlySet<string>>(new Set());
  const [owed, setOwed] = useState<number | null>(null);
  const [credit, setCredit] = useState(0);
  const [expected, setExpected] = useState<Expected | null>(null);
  const [amount, setAmount] = useState("");
  // The school honouring the early-payment reward after the window closed. A
  // decision somebody makes, which is why it is a switch and not a default.
  const [grantEarly, setGrantEarly] = useState(false);
  const [method, setMethod] = useState<PaymentMethod>("cash");
  // Debit or credit, asked for only once the money is on a card. The office
  // reconciles against the terminal at month end.
  const [cardType, setCardType] = useState<CardType>("debit");
  const [reference, setReference] = useState("");
  const [preview, setPreview] = useState<{ allocations: Payment[]; surcharges: Charge[]; unallocatedCents: number } | null>(null);
  const [receipt, setReceipt] = useState<DepositResult | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [attemptKey] = useState(newAttemptKey);
  const describe = useRef<Map<string, string>>(new Map());

  const name = `${student.firstName} ${student.lastName}`;
  const cents = pesosToCents(amount);
  const lines = expected?.lines ?? [];
  const chosenLines = lines.filter((l) => !unchosen.has(l.chargeId));
  const choice = lines.length > 0 && chosenLines.length < lines.length ? chosenLines.map((l) => l.chargeId) : undefined;
  const choiceKey = choice?.join(",") ?? "";
  const selectedCents = chosenLines.reduce((sum, l) => sum + l.totalCents, 0);
  // With a choice, the difference is measured against what was chosen.
  const target = choice !== undefined ? selectedCents : (expected?.amountCents ?? 0);
  const toggle = (chargeId: string): void => {
    const next = new Set(unchosen);
    if (next.has(chargeId)) next.delete(chargeId);
    else next.add(chargeId);
    setUnchosen(next);
    // The amount follows the choice until the cashier types another one.
    if (!amountEdited.current) {
      const cents = lines.filter((l) => !next.has(l.chargeId)).reduce((sum, l) => sum + l.totalCents, 0);
      prefilledCents.current = cents;
      setAmount(centsToPesosInput(cents));
    }
  };
  const ready = Number.isFinite(cents) && cents > 0 && !saving;

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
        // Pre-filled with what should be paid: the common case is a family
        // paying exactly that, and retyping it is a chance to mistype it.
        if (e.amountCents > 0 && !amountEdited.current) {
          prefilledCents.current = e.amountCents;
          setAmount(centsToPesosInput(e.amountCents));
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
    if (!dateOk) {
      setDateError(paymentDateError(paidOn, day));
      focusFirstInvalid();
      return;
    }
    if (!ready) {
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      const result = await registerDeposit(API_URL, {
        studentId: student.id,
        amountCents: cents,
        method,
        reference: reference.trim(),
        idempotencyKey: attemptKey,
        ...(sentDate !== "" ? { paidAt: sentDate } : {}),
        ...(choice !== undefined ? { chargeIds: choice } : {}),
        ...(grantEarly ? { grantEarlyPayment: true } : {}),
        ...(method === "card" ? { cardType } : {}),
      });
      setReceipt(result);
      onCollected();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo registrar el cobro.");
    } finally {
      setSaving(false);
    }
  }, [dateOk, ready, student.id, cents, method, cardType, reference, attemptKey, grantEarly, sentDate, paidOn, day, choice, focusFirstInvalid, onCollected]);

  return (
    <DrawerShell
      title={`Cobrar · ${name}`}
      subtitle={student.gradeGroup ?? ""}
      icon="card"
      size="lg"
      onClose={onClose}
      // El velo original de este cajón NO tenía onClick ni Esc, a propósito: es
      // la ventanilla, y el monto, la forma de pago y la referencia ya tecleados
      // no pueden irse con un clic fuera de lugar. La cáscara común cierra por
      // omisión, así que aquí hay que decirlo. El ✕ y Cancelar siguen cerrando.
      dismissible={false}
      padded={false}
    >
      <>
        {receipt !== null
          ? <Receipt result={receipt} describe={describe.current} onClose={onClose} />
          : (
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
                    Del año debe <b data-testid="amount-owed">{owed === null ? "…" : formatCents(owed)}</b>.
                  </p>
                  {credit > 0 && (
                    <p style={{ fontSize: 13.5, color: "var(--text-muted)", marginTop: 6 }}>
                      Tiene {formatCents(credit)} a favor.
                    </p>
                  )}
                </div>

                <DebtList lines={lines} unchosen={unchosen} onToggle={toggle} />

                <MoneyField
                  label="Monto recibido"
                  value={amount}
                  onChange={(v) => {
                    if (pesosToCents(v) !== prefilledCents.current) amountEdited.current = true;
                    setAmount(v);
                  }}
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

                {expected !== null && expected.discountCents === 0 && (
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
                    {preview.allocations.map((a) => (
                      <div key={a.id} style={{ display: "flex", justifyContent: "space-between", padding: "12px 16px", borderTop: "1px solid var(--border)" }}>
                        <span style={{ fontSize: 14.5, color: "var(--text)" }}>{label(a.chargeId, preview.surcharges, describe.current)}</span>
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

                <div>
                  <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 3 }}>Método</span>
                  <div style={{ display: "flex", gap: 8, marginTop: 8, flexWrap: "wrap" }}>
                    {METHODS.map((m) => {
                      const on = method === m.value;
                      return (
                        <button
                          key={m.value}
                          type="button"
                          onClick={() => setMethod(m.value)}
                          style={{ height: 44, padding: "0 18px", borderRadius: 99, fontSize: 14.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>
                          {m.label}
                        </button>
                      );
                    })}
                  </div>
                </div>

                {method === "card" && (
                  <div>
                    <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 3 }}>Tipo de tarjeta</span>
                    <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
                      {([["debit", "Débito"], ["credit", "Crédito"]] as [CardType, string][]).map(([value, label]) => {
                        const on = cardType === value;
                        return (
                          <button
                            key={value}
                            type="button"
                            onClick={() => setCardType(value)}
                            aria-label={label}
                            style={{ height: 44, padding: "0 18px", borderRadius: 99, fontSize: 14.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>
                            {label}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}

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

                <label style={{ display: "block" }}>
                  <span style={{ display: "inline-flex", alignItems: "baseline", gap: 6, marginLeft: 3 }}>
                    <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Referencia (opcional)</span>
                    <HelpTip id="cobro-ref-help" label="Referencia" text={REFERENCIA_AYUDA} />
                  </span>
                  <div style={{ marginTop: 8 }}>
                    <input value={reference} onChange={(e) => setReference(e.target.value)} aria-label="Referencia" aria-describedby="cobro-ref-help" style={{ ...field, height: 50, fontSize: 15.5, fontWeight: 400 }} />
                  </div>
                </label>

                {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>}
              </div>

              <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "18px 26px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>
                <Btn kind="ghost" type="button" onClick={onClose}>Cancelar</Btn>
                <div style={{ flex: 1 }} />
                <Btn icon="check" type="submit" style={{ opacity: ready ? 1 : 0.45, height: 52, fontSize: 16 }}>Cobrar</Btn>
              </div>
            </form>
          )}
      </>
    </DrawerShell>
  );
}

/**
 * What one allocation covered, in words. A surcharge created by this very
 * payment is not in the account read, so its description comes from the result
 * itself.
 */
function label(chargeId: string, surcharges: Charge[], known: Map<string, string>): string {
  const fresh = surcharges.find((c) => c.id === chargeId);
  if (fresh !== undefined) {
    return fresh.description;
  }
  return known.get(chargeId) ?? "Cargo";
}

/**
 * The receipt. A folio a parent can quote and a list they can check, printable
 * because a school hands over paper.
 */
function Receipt({ result, describe, onClose }: { result: DepositResult; describe: Map<string, string>; onClose: () => void }): ReactElement {
  return (
    <div data-testid="receipt" style={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
      <div style={{ flex: 1, overflowY: "auto", padding: "22px 26px", display: "flex", flexDirection: "column", gap: 18 }}>
        <div style={{ padding: "18px 20px", borderRadius: "var(--r-md)", background: "var(--ok-bg)", border: "1px solid var(--border)" }}>
          <span style={{ fontSize: 14, color: "var(--ok-fg)" }}>Cobrado</span>
          <div style={{ fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 34, color: "var(--ok-fg)", marginTop: 4 }}>
            {formatCents(result.deposit.amountCents)}
          </div>
          <p style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 8 }}>
            Folio {folio(result.deposit.id)}
            {result.replayed && " · ya estaba registrado, no se cobró de nuevo"}
          </p>
        </div>
        <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden" }}>
          {result.allocations.map((a) => (
            <div key={a.id} style={{ display: "flex", justifyContent: "space-between", padding: "12px 16px", borderTop: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14.5, color: "var(--text)" }}>{label(a.chargeId, result.surcharges, describe)}</span>
              <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15 }}>{formatCents(a.amountCents)}</span>
            </div>
          ))}
        </div>
        {result.unallocatedCents > 0 && (
          <p style={{ fontSize: 14, color: "var(--text-muted)", margin: 0 }}>
            Quedan {formatCents(result.unallocatedCents)} a favor de la familia.
          </p>
        )}
      </div>
      <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "18px 26px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>
        <Btn kind="ghost" type="button" onClick={() => window.print()}>Imprimir</Btn>
        <div style={{ flex: 1 }} />
        <Btn icon="check" type="button" onClick={onClose}>Listo</Btn>
      </div>
    </div>
  );
}

/** A folio a person can read aloud: the tail of the deposit id, uppercased. */
function folio(id: string): string {
  return id.replace(/-/g, "").slice(-8).toUpperCase();
}

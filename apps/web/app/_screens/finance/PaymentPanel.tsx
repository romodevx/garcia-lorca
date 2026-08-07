"use client";

import { useEffect, useState, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Btn } from "../../_design/Controls";
import type { Charge, PaymentMethod, RegisterPaymentInput } from "../../finance/api";
import { centsToPesosInput, formatCents, pesosToCents } from "./money";
import { ErrorAlert, RadioGroup, Section, SelectField, TextField } from "./parts";

const DEFAULT_METHOD: PaymentMethod = "cash";
const METHODS: readonly PaymentMethod[] = ["cash", "card", "transfer"];

/**
 * Mint a handle on ONE payment attempt. The key only has to be unique, never
 * unguessable, so the fallback is fine where `crypto.randomUUID` is missing —
 * it is exposed to secure contexts only, and the ventanilla may well be opened
 * over plain http on the school's LAN.
 */
function newAttemptKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export interface PaymentPanelProps {
  /** Collectable charges (a remaining balance, not cancelled). */
  charges: readonly Charge[];
  selectedChargeId: string;
  onSelectCharge: (chargeId: string) => void;
  studentName: (studentId: string) => string;
  /** Rejects with the gateway's message so this panel can show WHY it bounced. */
  onSubmit: (input: RegisterPaymentInput) => Promise<void>;
}

/**
 * Ventanilla (window) payment form — the fewest-clicks path: the charge arrives
 * preselected from the debts table, the amount is prefilled to its remaining
 * balance, and cash is the default method, so collecting is a single click.
 *
 * The human types PESOS; `pesosToCents` converts once, here, and only integer
 * cents ever reach the api client.
 *
 * Every submit carries an idempotency key identifying the ATTEMPT, not the
 * click: a submit that fails or hangs is retried under the same key, so a
 * double-click — or a response lost after the server already banked the money —
 * cannot charge the family twice. The key is renewed once a collection lands,
 * and whenever the cashier moves to another charge, because those are genuinely
 * new attempts.
 */
export function PaymentPanel({ charges, selectedChargeId, onSelectCharge, studentName, onSubmit }: PaymentPanelProps): ReactElement {
  const t = useTranslations("finance");
  const [amount, setAmount] = useState("");
  const [method, setMethod] = useState<PaymentMethod>(DEFAULT_METHOD);
  const [reference, setReference] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [attemptKey, setAttemptKey] = useState(newAttemptKey);

  const selected = charges.find((charge) => charge.id === selectedChargeId);
  const remainingCents = selected?.remainingCents;

  // Prefill (and re-prefill) the amount from the selected charge's balance.
  useEffect(() => {
    setAmount(remainingCents !== undefined ? centsToPesosInput(remainingCents) : "");
  }, [selectedChargeId, remainingCents]);

  // Another charge is another attempt — never dedupe one student's payment
  // against another's.
  useEffect(() => {
    setAttemptKey(newAttemptKey());
  }, [selectedChargeId]);

  const submit = async (): Promise<void> => {
    setError(undefined);
    if (selected === undefined) {
      setError(t("error.chargeRequired"));
      return;
    }
    const amountCents = pesosToCents(amount);
    if (!Number.isFinite(amountCents) || amountCents <= 0) {
      setError(t("error.amount"));
      return;
    }
    setPending(true);
    try {
      await onSubmit({ chargeId: selected.id, amountCents, method, reference, idempotencyKey: attemptKey });
      setReference("");
      // The money is in: anything the cashier collects next is a new attempt.
      setAttemptKey(newAttemptKey());
    } catch (cause) {
      setError(t("error.payment", { detail: cause instanceof Error ? cause.message : String(cause) }));
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title={t("section.payment")} subtitle={t("hint.overpay")}>
      <div style={{ display: "grid", gap: 14, maxWidth: 620 }}>
        <SelectField
          label={t("field.charge")}
          value={selectedChargeId}
          options={charges.map((charge) => ({
            value: charge.id,
            label: `${studentName(charge.studentId)} · ${charge.description} · ${formatCents(charge.remainingCents)}`,
          }))}
          onChange={onSelectCharge}
          placeholder={t("option.choose")}
        />
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 14 }}>
          <TextField label={t("field.amount")} value={amount} onChange={setAmount} placeholder="0.00" />
          <TextField label={t("field.reference")} value={reference} onChange={setReference} />
        </div>
        <RadioGroup
          legend={t("field.method")}
          value={method}
          onChange={setMethod}
          options={METHODS.map((option) => ({ value: option, label: t(`method.${option}`) }))}
        />
        <p style={{ fontSize: 12.5, color: "var(--text-faint)", margin: 0 }}>{t("hint.amountPesos")}</p>
        <div>
          <Btn icon="card" onClick={() => void submit()} disabled={pending}>
            {t("action.pay")}
          </Btn>
        </div>
        {error !== undefined && <ErrorAlert message={error} />}
      </div>
    </Section>
  );
}

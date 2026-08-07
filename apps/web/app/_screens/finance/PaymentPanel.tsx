"use client";

import { useEffect, useState, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Btn } from "../../_design/Controls";
import type { Charge, RegisterPaymentInput } from "../../finance/api";
import { centsToPesosInput, ErrorAlert, formatCents, pesosToCents, RadioGroup, Section, SelectField, TextField } from "./parts";

const DEFAULT_METHOD = "cash";

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
 */
export function PaymentPanel({ charges, selectedChargeId, onSelectCharge, studentName, onSubmit }: PaymentPanelProps): ReactElement {
  const t = useTranslations("finance");
  const [amount, setAmount] = useState("");
  const [method, setMethod] = useState(DEFAULT_METHOD);
  const [reference, setReference] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const selected = charges.find((charge) => charge.id === selectedChargeId);
  const remainingCents = selected?.remainingCents;

  // Prefill (and re-prefill) the amount from the selected charge's balance.
  useEffect(() => {
    setAmount(remainingCents !== undefined ? centsToPesosInput(remainingCents) : "");
  }, [selectedChargeId, remainingCents]);

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
      await onSubmit({ chargeId: selected.id, amountCents, method, reference });
      setReference("");
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
        />
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 14 }}>
          <TextField label={t("field.amount")} value={amount} onChange={setAmount} placeholder="0.00" />
          <TextField label={t("field.reference")} value={reference} onChange={setReference} />
        </div>
        <RadioGroup
          legend={t("field.method")}
          value={method}
          onChange={setMethod}
          options={[
            { value: "cash", label: t("method.cash") },
            { value: "card", label: t("method.card") },
            { value: "transfer", label: t("method.transfer") },
          ]}
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

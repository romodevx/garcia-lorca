"use client";

import { useEffect, useState, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Btn } from "../../_design/Controls";
import type { Cycle, Group } from "../../config/api";
import type { ChargeConcept, GenerateChargesInput } from "../../finance/api";
import { pesosToCents } from "../../_lib/money";
import { DateField, SelectField, TextField } from "../../_design/form";
import { ErrorAlert, Section } from "./parts";

export interface GeneratePanelProps {
  concepts: readonly ChargeConcept[];
  cycles: readonly Cycle[];
  cycleId: string;
  onCycleChange: (cycleId: string) => void;
  /** Groups of the chosen cycle; loaded by the orchestrator. */
  groups: readonly Group[];
  onSubmit: (input: GenerateChargesInput) => Promise<void>;
}

/**
 * Bulk charge generation: one concept across a cycle (optionally a single
 * group). Idempotent server-side — a re-run reports the already-charged students
 * as skipped. An empty amount means "use the concept's default", which the
 * gateway reads as a 0 override.
 */
export function GeneratePanel({ concepts, cycles, cycleId, onCycleChange, groups, onSubmit }: GeneratePanelProps): ReactElement {
  const t = useTranslations("finance");
  const [conceptId, setConceptId] = useState("");
  const [groupId, setGroupId] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [amount, setAmount] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  // A group belongs to one cycle: changing the cycle invalidates the selection.
  useEffect(() => setGroupId(""), [cycleId]);

  const submit = async (): Promise<void> => {
    setError(undefined);
    const override = amount.trim() === "" ? 0 : pesosToCents(amount);
    if (!Number.isFinite(override) || override < 0) {
      setError(t("error.amount"));
      return;
    }
    setPending(true);
    try {
      await onSubmit({ conceptId, cycleId, groupId, amountCentsOverride: override, dueDate });
    } catch (cause) {
      setError(t("error.generate", { detail: cause instanceof Error ? cause.message : String(cause) }));
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title={t("section.generate")} subtitle={t("hint.groupOptional")}>
      <div style={{ display: "grid", gap: 14 }}>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 14 }}>
          <SelectField
            label={t("field.concept")}
            value={conceptId}
            options={concepts.map((concept) => ({ value: concept.id, label: concept.name }))}
            onChange={setConceptId}
            help={t("help.concept")}
          />
          <SelectField
            label={t("field.cycle")}
            value={cycleId}
            options={cycles.map((cycle) => ({ value: cycle.id, label: cycle.name }))}
            onChange={onCycleChange}
            help={t("help.cycle")}
          />
          <SelectField
            label={t("field.group")}
            value={groupId}
            options={groups.map((group) => ({ value: group.id, label: group.name }))}
            onChange={setGroupId}
            help={t("help.group")}
            disabled={cycleId === ""}
          />
          <DateField label={t("field.dueDate")} value={dueDate} onChange={setDueDate} />
          <TextField label={t("field.amountOverride")} value={amount} onChange={setAmount} help={t("help.amountOverride")} />
        </div>
        <div>
          <Btn icon="plus" onClick={() => void submit()} disabled={pending}>
            {t("action.generate")}
          </Btn>
        </div>
        {error !== undefined && <ErrorAlert message={error} />}
      </div>
    </Section>
  );
}

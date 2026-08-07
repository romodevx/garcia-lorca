"use client";

import { useState, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Btn } from "../../_design/Controls";
import type { ChargeConcept, CreateConceptInput, UpdateConceptInput } from "../../finance/api";
import {
  CheckBoxField,
  EmptyHint,
  ErrorAlert,
  formatCents,
  pesosToCents,
  RadioGroup,
  Section,
  TextField,
  centsToPesosInput,
} from "./parts";

/** Blank draft = create mode; a draft with an id = editing that concept. */
interface Draft {
  id?: string;
  name: string;
  amount: string;
  recurrence: string;
  active: boolean;
}

const EMPTY_DRAFT: Draft = { name: "", amount: "", recurrence: "monthly", active: true };

export interface ConceptsPanelProps {
  concepts: readonly ChargeConcept[];
  includeInactive: boolean;
  onIncludeInactiveChange: (include: boolean) => void;
  onCreate: (input: CreateConceptInput) => Promise<void>;
  onUpdate: (id: string, input: UpdateConceptInput) => Promise<void>;
}

/**
 * Charge concepts (colegiatura, inscripción…): one form that creates or edits,
 * plus the list. Amounts are typed in PESOS and converted to integer cents
 * before they reach the api client. Errors from the gateway are shown verbatim.
 */
export function ConceptsPanel({
  concepts,
  includeInactive,
  onIncludeInactiveChange,
  onCreate,
  onUpdate,
}: ConceptsPanelProps): ReactElement {
  const t = useTranslations("finance");
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const editing = draft.id !== undefined;

  const patch = (fields: Partial<Draft>): void => setDraft((current) => ({ ...current, ...fields }));

  const submit = async (): Promise<void> => {
    setError(undefined);
    const defaultAmountCents = pesosToCents(draft.amount);
    if (!Number.isFinite(defaultAmountCents) || defaultAmountCents <= 0) {
      setError(t("error.amount"));
      return;
    }
    setPending(true);
    try {
      if (draft.id !== undefined) {
        await onUpdate(draft.id, { name: draft.name, defaultAmountCents, recurrence: draft.recurrence, active: draft.active });
      } else {
        await onCreate({ name: draft.name, defaultAmountCents, recurrence: draft.recurrence });
      }
      setDraft(EMPTY_DRAFT);
    } catch (cause) {
      setError(t("error.concept", { detail: cause instanceof Error ? cause.message : String(cause) }));
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title={t("section.concepts")} subtitle={t("hint.amountPesos")}>
      <div style={{ display: "grid", gap: 14 }}>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 14 }}>
          <TextField label={t("field.name")} value={draft.name} onChange={(name) => patch({ name })} />
          <TextField label={t("field.amount")} value={draft.amount} onChange={(amount) => patch({ amount })} placeholder="0.00" />
        </div>
        <div style={{ display: "flex", gap: 14, flexWrap: "wrap", alignItems: "flex-end" }}>
          <RadioGroup
            legend={t("field.recurrence")}
            value={draft.recurrence}
            onChange={(recurrence) => patch({ recurrence })}
            options={[
              { value: "once", label: t("recurrence.once") },
              { value: "monthly", label: t("recurrence.monthly") },
            ]}
          />
          {editing && <CheckBoxField label={t("field.active")} checked={draft.active} onChange={(active) => patch({ active })} />}
        </div>
        <div style={{ display: "flex", gap: 12 }}>
          <Btn icon={editing ? "check" : "plus"} onClick={() => void submit()} disabled={pending}>
            {editing ? t("action.saveConcept") : t("action.createConcept")}
          </Btn>
        </div>
        {error !== undefined && <ErrorAlert message={error} />}

        <div style={{ display: "flex", justifyContent: "flex-end" }}>
          <CheckBoxField label={t("option.includeInactive")} checked={includeInactive} onChange={onIncludeInactiveChange} />
        </div>

        {concepts.length === 0 ? (
          <EmptyHint>{t("state.empty.concepts")}</EmptyHint>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "grid", gap: 8 }}>
            {concepts.map((concept) => (
              <li
                key={concept.id}
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 14,
                  padding: "12px 16px",
                  borderRadius: "var(--r-md)",
                  border: "1px solid var(--border)",
                  background: concept.active ? "var(--surface)" : "var(--surface-3)",
                }}
              >
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontWeight: 600, fontSize: 15, color: "var(--text)" }}>{concept.name}</div>
                  <div style={{ fontSize: 13, color: "var(--text-muted)" }}>
                    {formatCents(concept.defaultAmountCents)} ·{" "}
                    {concept.recurrence === "once" ? t("recurrence.once") : t("recurrence.monthly")}
                    {!concept.active && ` · ${t("state.inactive")}`}
                  </div>
                </div>
                <Btn
                  kind="ghost"
                  icon="settings"
                  onClick={() =>
                    setDraft({
                      id: concept.id,
                      name: concept.name,
                      amount: centsToPesosInput(concept.defaultAmountCents),
                      recurrence: concept.recurrence,
                      active: concept.active,
                    })
                  }
                  style={{ height: 40, fontSize: 14 }}
                >
                  {t("action.editConcept", { name: concept.name })}
                </Btn>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Section>
  );
}

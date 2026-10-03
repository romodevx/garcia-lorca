"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { MoneyField, TextField, useFocusFirstInvalid } from "../../_design/form";
import { API_URL } from "../../api-url";
import { createScholarship, SCHOLARSHIP_KINDS, updateScholarship, type Scholarship, type ScholarshipKind, type ScholarshipTarget } from "../../scholarships/api";
import { KIND_LABEL, scholarshipTitle } from "../../scholarships/format";
import { ErrorAlert, RadioGroup } from "../config/parts";
import { PercentField } from "./PercentField";
import { draftFromScholarship, emptyDraft, readDraft, valueError, type DraftErrors, type ScholarshipDraft } from "./scholarshipDraft";
import { TargetChecklist } from "./TargetChecklist";

const KIND_OPTIONS = SCHOLARSHIP_KINDS.map((k) => ({ value: k, label: KIND_LABEL[k] }));

const VALUE_FIELD: Readonly<Record<ScholarshipKind, { label: string; help: string }>> = {
  percent: { label: "Porcentaje de descuento", help: "La parte del precio que se descuenta, de 0.01 a 100, con hasta dos decimales." },
  fixed_off: { label: "Monto de descuento", help: "Lo que se le quita al precio, en pesos. Nunca deja el cargo debajo de cero." },
  fixed_price: { label: "La familia paga", help: "Lo que paga la familia en lugar del precio, en pesos. Si el precio es menor, paga el precio." },
};

export interface ScholarshipDrawerProps {
  /** Absent: a new scholarship. */
  scholarship?: Scholarship;
  targets: readonly ScholarshipTarget[];
  onClose: () => void;
  onSaved: (message: string) => void;
}

function without(errors: DraftErrors, key: keyof DraftErrors): DraftErrors {
  if (errors[key] === undefined) return errors;
  const rest = { ...errors };
  delete rest[key];
  return rest;
}

/**
 * A scholarship's create/edit form (T18). Five fields, so a dialog
 * (ui-forms-and-modals): one column, labels above, a badly written value said
 * on blur, what is missing said on save with focus on the first, the save
 * button never disabled. DrawerShell gives Esc, Enter and focus return.
 */
export function ScholarshipDrawer({ scholarship, targets, onClose, onSaved }: ScholarshipDrawerProps): ReactElement {
  const [draft, setDraft] = useState<ScholarshipDraft>(() => (scholarship === undefined ? emptyDraft() : draftFromScholarship(scholarship)));
  const [errors, setErrors] = useState<DraftErrors>({});
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const focusFirstInvalid = useFocusFirstInvalid();

  const set = <K extends keyof ScholarshipDraft>(key: K, value: ScholarshipDraft[K]): void => {
    setDraft((d) => ({ ...d, [key]: value }));
    if (key === "name" || key === "targets") setErrors((e) => without(e, key));
  };

  // Accused on blur, forgiven by the keystroke that makes it right.
  const changeValue = (value: string): void => {
    setDraft((d) => ({ ...d, value }));
    if (valueError(draft.kind, value) === undefined) setErrors((e) => without(e, "value"));
  };
  const blurValue = (): void => {
    const problem = valueError(draft.kind, draft.value);
    if (problem !== undefined) setErrors((e) => ({ ...e, value: problem }));
  };
  // "50" means 50 % or $50: a value never silently changes meaning with the kind.
  const changeKind = (kind: ScholarshipKind): void => {
    setDraft((d) => ({ ...d, kind, value: "" }));
    setErrors((e) => without(e, "value"));
  };

  const submit = async (): Promise<void> => {
    if (pending) return;
    const { input, errors: found } = readDraft(draft);
    setErrors(found);
    if (input === undefined) {
      focusFirstInvalid();
      return;
    }
    setPending(true);
    setFailure(undefined);
    try {
      const saved = scholarship === undefined ? await createScholarship(API_URL, input) : await updateScholarship(API_URL, scholarship.id, input);
      onSaved(`${scholarshipTitle(saved.name)} guardada`);
    } catch (err: unknown) {
      setFailure(err instanceof Error ? err.message : "No se pudo guardar la beca.");
    } finally {
      setPending(false);
    }
  };

  const valueField = VALUE_FIELD[draft.kind];
  const valueProps = { label: valueField.label, help: valueField.help, required: true, value: draft.value, error: errors.value, onChange: changeValue, onBlur: blurValue };
  const holders = scholarship?.assignedCount ?? 0;
  const deactivating = scholarship?.active === true && !draft.active && holders > 0;

  return (
    <DrawerShell
      title={scholarship === undefined ? "Nueva beca" : `Editar ${scholarship.name}`}
      subtitle="Qué descuenta y a qué se aplica"
      icon="scholarship"
      onClose={onClose}
      onSubmit={() => void submit()}
      footer={
        <>
          <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          <Btn type="submit" icon="check">{pending ? "Guardando…" : "Guardar beca"}</Btn>
        </>
      }
    >
      <div style={{ display: "grid", gap: 18 }}>
        <TextField label="Nombre" required uppercase value={draft.name} error={errors.name} onChange={(v) => set("name", v)} />
        <RadioGroup legend="Tipo de beca" options={KIND_OPTIONS} value={draft.kind} onChange={changeKind} />
        {draft.kind === "percent" ? <PercentField {...valueProps} /> : <MoneyField {...valueProps} />}
        <TargetChecklist targets={targets} chosen={draft.targets} error={errors.targets} onChange={(next) => set("targets", next)} />
        <div>
          <label style={{ display: "flex", alignItems: "center", gap: 10, minHeight: 44, cursor: "pointer" }}>
            <input type="checkbox" checked={draft.active} onChange={(e) => set("active", e.target.checked)} style={{ width: 20, height: 20, accentColor: "var(--forest)" }} />
            <span style={{ fontSize: 15, color: "var(--text)" }}>Activa (se puede asignar y se aplica)</span>
          </label>
          {deactivating && (
            <p role="status" style={{ margin: "4px 0 0", fontSize: 13.5, color: "var(--warn-fg)" }}>
              {`Al guardar se le quita a sus ${holders} alumnos: sus cargos pendientes vuelven al precio completo. Lo ya pagado no cambia.`}
            </p>
          )}
        </div>
        {failure !== undefined && <ErrorAlert message={failure} />}
      </div>
    </DrawerShell>
  );
}

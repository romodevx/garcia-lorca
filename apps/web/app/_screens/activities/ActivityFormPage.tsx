"use client";

import { useState, type ReactElement, type ReactNode } from "react";

import { Btn } from "../../_design/Controls";
import { CreatePage } from "../../_design/CreatePage";
import { DateField, MoneyField, SelectField, TextField, useFocusFirstInvalid } from "../../_design/form";
import { API_URL } from "../../api-url";
import { createActivity, PERIODICITIES, updateActivity, type Activity, type Periodicity } from "../../activities/api";
import { PERIODICITY_LABEL } from "../../activities/format";
import { ErrorAlert } from "../config/parts";
import {
  draftFromActivity, emptyDraft, newOption, newSlot, readDraft, slotKey,
  type ActivityDraft, type OptionDraft, type SlotDraft,
} from "./activityDraft";
import { PriceOptionRows } from "./PriceOptionRows";
import { ScheduleRows } from "./ScheduleRows";

const PERIODICITY_OPTIONS = PERIODICITIES.map((p) => ({ value: p, label: PERIODICITY_LABEL[p] }));
const END_BEFORE_START = "La hora de fin va después del inicio.";

export interface ActivityFormPageProps {
  /** Absent: a new activity. */
  activity?: Activity;
  /** The office's day: the first billing date's default. */
  today: string;
  onClose: () => void;
  onSaved: (message: string) => void;
}

function without(errors: Record<string, string>, ...keys: string[]): Record<string, string> {
  if (!keys.some((k) => errors[k] !== undefined)) return errors;
  const rest = { ...errors };
  for (const k of keys) delete rest[k];
  return rest;
}

/**
 * An activity's create/edit form (T17). More than eight fields with
 * repeatable rows, so it is a PAGE (ui-forms-and-modals): one column, labels
 * above, problems marked on save with focus on the first, the save button
 * never disabled.
 */
export function ActivityFormPage({ activity, today, onClose, onSaved }: ActivityFormPageProps): ReactElement {
  const [draft, setDraft] = useState<ActivityDraft>(() => (activity === undefined ? emptyDraft(today) : draftFromActivity(activity)));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const focusFirstInvalid = useFocusFirstInvalid();

  const set = <K extends keyof ActivityDraft>(key: K, value: ActivityDraft[K]): void => {
    setDraft((d) => ({ ...d, [key]: value }));
    setErrors((e) => without(e, key));
  };

  const changeSlot = (i: number, patch: Partial<SlotDraft>): void => {
    const next = { ...(draft.slots[i] as SlotDraft), ...patch };
    setDraft((d) => ({ ...d, slots: d.slots.map((s, j) => (j === i ? next : s)) }));
    // A select is complete the moment it is chosen: the hours are judged then.
    setErrors((e) => {
      const cleared = without(e, ...Object.keys(patch).map((k) => slotKey(i, k as "weekday" | "start" | "end")));
      const backwards = next.start !== "" && next.end !== "" && next.end <= next.start;
      return backwards ? { ...cleared, [slotKey(i, "end")]: END_BEFORE_START } : without(cleared, slotKey(i, "end"));
    });
  };

  const changeOption = (i: number, patch: Partial<OptionDraft>): void => {
    setDraft((d) => ({ ...d, options: d.options.map((o, j) => (j === i ? { ...o, ...patch } : o)) }));
    setErrors((e) => without(e, ...Object.keys(patch).map((k) => `option-${i}-${k}`)));
  };

  // Removing a row renumbers the keys below it: its errors are re-said on save.
  const removeSlot = (i: number): void => { setDraft((d) => ({ ...d, slots: d.slots.filter((_, j) => j !== i) })); setErrors({}); };
  const removeOption = (i: number): void => { setDraft((d) => ({ ...d, options: d.options.filter((_, j) => j !== i) })); setErrors({}); };

  const submit = async (): Promise<void> => {
    const { input, errors: found } = readDraft(draft);
    setErrors(found);
    if (input === undefined) {
      focusFirstInvalid();
      return;
    }
    setPending(true);
    setFailure(undefined);
    try {
      const saved = activity === undefined ? await createActivity(API_URL, input) : await updateActivity(API_URL, activity.id, input);
      onSaved(`Actividad ${saved.name} guardada`);
    } catch (err: unknown) {
      setFailure(err instanceof Error ? err.message : "No se pudo guardar la actividad.");
    } finally {
      setPending(false);
    }
  };

  return (
    <CreatePage
      breadcrumb={["Actividades"]}
      title={activity === undefined ? "Nueva actividad" : `Editar ${activity.name}`}
      subtitle="Horario, cobro y precios"
      footer={
        <>
          <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          <Btn icon="check" onClick={() => void submit()}>{pending ? "Guardando…" : "Guardar actividad"}</Btn>
        </>
      }
    >
      <Section title="Actividad">
        <TextField label="Nombre" required uppercase value={draft.name} error={errors.name} onChange={(v) => set("name", v)} />
        <label style={{ display: "flex", alignItems: "center", gap: 10, minHeight: 44, cursor: "pointer" }}>
          <input type="checkbox" checked={draft.active} onChange={(e) => set("active", e.target.checked)} style={{ width: 20, height: 20, accentColor: "var(--forest)" }} />
          <span style={{ fontSize: 15, color: "var(--text)" }}>Activa (se puede inscribir y se cobra)</span>
        </label>
      </Section>

      <Section title="Horario" note="varios días, cada uno con su hora">
        <ScheduleRows slots={draft.slots} errors={errors} onChange={changeSlot}
          onAdd={() => set("slots", [...draft.slots, newSlot()])} onRemove={removeSlot} />
      </Section>

      <Section title="Cobro">
        <SelectField label="Periodicidad" required value={draft.periodicity} options={PERIODICITY_OPTIONS}
          onChange={(v) => set("periodicity", (v === "" ? "monthly" : v) as Periodicity)}
          help="Cada cuánto se cobra. Una vez por ciclo es un solo cargo, en la fecha del primer cobro." />
        <DateField label="Primer cobro" required value={draft.billingStartsOn} error={errors.billingStartsOn} onChange={(v) => set("billingStartsOn", v)}
          help="El día en que se genera el primer cargo. Los siguientes se repiten el mismo día de cada mes o de cada semana." />
        <DateField label="Primera fecha límite" required value={draft.firstDueOn} error={errors.firstDueOn} min={draft.billingStartsOn} onChange={(v) => set("firstDueOn", v)}
          help="Hasta cuándo se paga el primer periodo sin recargo. Los siguientes guardan la misma distancia." />
        <MoneyField label="Recargo (monto fijo, 0 = sin recargo)" value={draft.lateFee} error={errors.lateFee} onChange={(v) => set("lateFee", v)}
          help="Se suma una sola vez al periodo vencido y sin pagar. Vacío o 0: sin recargo." />
      </Section>

      <Section title="Precios" note="por días a la semana o cuota fija">
        <PriceOptionRows options={draft.options} errors={errors} onChange={changeOption}
          onAdd={() => set("options", [...draft.options, newOption()])} onRemove={removeOption} />
      </Section>

      {failure !== undefined && <ErrorAlert message={failure} />}
    </CreatePage>
  );
}

function Section({ title, note, children }: { title: string; note?: string; children: ReactNode }): ReactElement {
  return (
    <section style={{ display: "grid", gap: 14, padding: "18px 20px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)" }}>
      <h2 style={{ fontSize: 17, fontWeight: 800, color: "var(--text)" }}>
        {title}
        {note !== undefined && <span style={{ marginLeft: 8, fontSize: 13.5, fontWeight: 500, color: "var(--text-muted)" }}>{note}</span>}
      </h2>
      {children}
    </section>
  );
}

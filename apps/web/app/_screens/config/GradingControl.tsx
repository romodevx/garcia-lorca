"use client";

import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Toast } from "../../_design/Toast";
import { NumberField, useFocusFirstInvalid } from "../../_design/form";
import type { Level, LevelGrading } from "../../config/api";
import { DEFAULT_GRADING, fromDraft, gradingErrors, toDraft, type GradingDraft, type GradingErrors, type GradingField } from "./gradingDraft";
import { ErrorAlert, RadioGroup } from "./parts";
import { useSerialToggle } from "./useSerialToggle";

const TOAST_MS = 2600;
const DECIMAL_OPTIONS = [
  { value: "0", label: "Sin decimales" },
  { value: "1", label: "1 decimal" },
  { value: "2", label: "2 decimales" },
] as const;

export interface GradingControlProps {
  level: Level;
  onChange: (levelId: string, grading: LevelGrading) => Promise<void>;
}

/**
 * How a level grades, under its attendance mode: whether it carries a numeric
 * grade at all (preescolar usually does not — the toggle saves at once), and
 * its scale. The scale saves with one button; min < passing ≤ max is checked
 * on leaving each field and again on saving. The toggle sends the last SAVED
 * scale — never a half-typed one — one request at a time, and goes back on failure.
 */
export function GradingControl({ level, onChange }: GradingControlProps): ReactElement {
  const initial = level.grading ?? DEFAULT_GRADING;
  const [draft, setDraft] = useState<GradingDraft>(() => toDraft(initial));
  const [errors, setErrors] = useState<GradingErrors>({});
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const timer = useRef<number | undefined>(undefined);
  const lastSaved = useRef<LevelGrading>(initial);
  const focusFirstInvalid = useFocusFirstInvalid();

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const persist = useCallback(async (grading: LevelGrading): Promise<boolean> => {
    setFailure(undefined);
    try {
      await onChange(level.id, grading);
      lastSaved.current = grading;
      window.clearTimeout(timer.current);
      setToast(`${level.name}: calificación guardada`);
      timer.current = window.setTimeout(() => setToast(undefined), TOAST_MS);
      return true;
    } catch (err) {
      setFailure(`No se pudo guardar la calificación del nivel: ${err instanceof Error ? err.message : "reintenta"}`);
      return false;
    }
  }, [level.id, level.name, onChange]);

  const saveToggle = useCallback((next: boolean) => persist({ ...lastSaved.current, enabled: next }), [persist]);
  const { on: enabled, toggle } = useSerialToggle(initial.enabled, saveToggle);

  const edit = (field: keyof GradingDraft, value: string): void => {
    const next = { ...draft, [field]: value };
    setDraft(next);
    // A shown error clears the moment the field is right; nothing new is raised while typing.
    if (field !== "decimals" && errors[field] !== undefined && gradingErrors(next)[field] === undefined) {
      setErrors((prev) => ({ ...prev, [field]: undefined }));
    }
  };

  const check = (field: GradingField): void => setErrors((prev) => ({ ...prev, [field]: gradingErrors(draft)[field] }));

  const submit = (e: FormEvent): void => {
    e.preventDefault();
    const found = gradingErrors(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      focusFirstInvalid();
      return;
    }
    void persist(fromDraft(draft, true));
  };

  return (
    <div style={{ display: "grid", gap: 10, margin: "0 0 16px" }}>
      <label style={{ display: "inline-flex", alignItems: "center", gap: 10, minHeight: 44, fontSize: 15, fontWeight: 600, color: "var(--text)", cursor: "pointer" }}>
        <input type="checkbox" checked={enabled} onChange={(e) => toggle(e.target.checked)} style={{ width: 20, height: 20, accentColor: "var(--forest)" }} />
        Lleva calificación numérica
      </label>
      {enabled && (
        <form aria-label={`Calificación de ${level.name}`} onSubmit={submit} noValidate style={{ display: "grid", gap: 12, maxWidth: 360 }}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
            <NumberField label="Calificación mínima" value={draft.min} onChange={(v) => edit("min", v)} onBlur={() => check("min")} error={errors.min} />
            <NumberField label="Calificación máxima" value={draft.max} onChange={(v) => edit("max", v)} onBlur={() => check("max")} error={errors.max} />
          </div>
          <RadioGroup legend="Decimales" options={DECIMAL_OPTIONS} value={draft.decimals as (typeof DECIMAL_OPTIONS)[number]["value"]} onChange={(v) => edit("decimals", v)} />
          <NumberField label="Mínimo aprobatorio" value={draft.passing} onChange={(v) => edit("passing", v)} onBlur={() => check("passing")} error={errors.passing} help="Debajo de esta calificación el alumno reprueba el mes." />
          <div>
            <Btn type="submit" icon="check">Guardar calificación</Btn>
          </div>
        </form>
      )}
      {failure !== undefined && <ErrorAlert message={failure} />}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </div>
  );
}

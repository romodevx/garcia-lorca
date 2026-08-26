"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { ConfigError, type Cycle, type CreateCycleInput } from "../../config/api";
import { DateField, EmptyHint, ErrorAlert, Section, SelectChip, TextField } from "./parts";

export interface CycleBarProps {
  cycles: readonly Cycle[];
  selectedCycleId: string | undefined;
  onSelect: (id: string) => void;
  onCreate: (input: CreateCycleInput) => Promise<void>;
  /** Puts the selected draft year into service, standing down the previous one. */
  onActivate: (id: string) => Promise<void>;
}

/**
 * What went wrong, in words that point at a fix.
 *
 * The whole message used to be "Intenta de nuevo", which for the commonest
 * failure — a cycle name the school already used — is advice that can never
 * work. A person following it clicks the same button forever.
 */
export function cycleFailureMessage(err: unknown): string {
  if (err instanceof ConfigError) {
    if (err.isConflict) return "Ya existe un ciclo con ese nombre. Usa otro.";
    if (err.isInvalid) return "Revisa las fechas: el fin del ciclo debe ser posterior al inicio.";
    if (err.status === 401) return "Tu sesión expiró. Vuelve a entrar.";
    if (err.status === 403) return "Tu cuenta no tiene permiso para crear ciclos.";
    if (err.status === 404) return "El módulo de configuración está apagado.";
  }
  return "No pudimos crear el ciclo. Revisa tu conexión e intenta de nuevo.";
}

const STATUS_LABEL: Record<string, string> = {
  draft: "Borrador",
  active: "Activo",
  closed: "Cerrado",
};

/** Pick an existing cycle or create one (status omitted → server defaults draft). */
export function CycleBar({ cycles, selectedCycleId, onSelect, onCreate, onActivate }: CycleBarProps): ReactElement {
  const [name, setName] = useState("");
  const [startsOn, setStartsOn] = useState("");
  const [endsOn, setEndsOn] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [activating, setActivating] = useState(false);

  const selected = cycles.find((c) => c.id === selectedCycleId);

  const activate = async (): Promise<void> => {
    if (selected === undefined) return;
    setActivating(true);
    setError(undefined);
    try {
      await onActivate(selected.id);
    } catch (err) {
      setError(cycleFailureMessage(err));
    } finally {
      setActivating(false);
    }
  };

  const canSubmit = name.trim() !== "" && startsOn !== "" && endsOn !== "" && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ name: name.trim(), startsOn, endsOn });
      setName("");
      setStartsOn("");
      setEndsOn("");
    } catch (err) {
      setError(cycleFailureMessage(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Ciclo escolar" subtitle="Elige el ciclo a configurar o crea uno nuevo.">
      {cycles.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 18 }}>
          {cycles.map((c) => (
            <SelectChip key={c.id} selected={c.id === selectedCycleId} onClick={() => onSelect(c.id)}>
              {c.name} · {STATUS_LABEL[c.status] ?? c.status}
            </SelectChip>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay ciclos. Crea el primero para empezar.</EmptyHint>
      )}

      {/*
        A cycle is created as a draft, and until this button existed nothing could
        move it — so every school year read "Borrador" while the panel quietly
        resolved "the active cycle" to whichever came first. That fallback is
        right exactly while a school has one year and wrong from the second on.
      */}
      {selected !== undefined && (
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 18, flexWrap: "wrap" }}>
          {selected.status === "active" ? (
            <p style={{ fontSize: 14, color: "var(--text-muted)", margin: 0 }}>
              <strong>{selected.name}</strong> es el ciclo en curso.
            </p>
          ) : (
            <>
              <Btn icon="check" onClick={() => void activate()} disabled={activating}>
                {activating ? "Activando…" : "Activar ciclo"}
              </Btn>
              <p style={{ fontSize: 13.5, color: "var(--text-muted)", margin: 0 }}>
                El colegio trabaja sobre el ciclo activo: inscripciones, grupos y colegiaturas.
              </p>
            </>
          )}
        </div>
      )}

      <form aria-label="Agregar ciclo" onSubmit={submit}>
        <div style={{ display: "grid", gridTemplateColumns: "1.4fr 1fr 1fr auto", gap: 12, alignItems: "end" }}>
          <TextField label="Nombre del ciclo" value={name} onChange={setName} placeholder="2026-2027" />
          <DateField label="Inicio del ciclo" value={startsOn} onChange={setStartsOn} />
          <DateField label="Fin del ciclo" value={endsOn} onChange={setEndsOn} />
          <Btn type="submit" icon="plus" disabled={!canSubmit}>
            Crear ciclo
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

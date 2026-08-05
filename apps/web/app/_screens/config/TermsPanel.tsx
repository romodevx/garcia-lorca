"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateTermInput, Cycle, Level, Term } from "../../config/api";
import { DateField, EmptyHint, ErrorAlert, NumberField, Section, TextField } from "./parts";

export interface TermsPanelProps {
  cycle: Cycle;
  level: Level;
  terms: readonly Term[];
  onCreate: (input: CreateTermInput) => Promise<void>;
}

/** Suggests the next term ordinal from existing terms. */
function nextOrdinal(terms: readonly Term[]): number {
  return terms.reduce((max, t) => Math.max(max, t.ordinal), 0) + 1;
}

/** Terms (periodos) for a (cycle, level), with an ordinal suggestion + dates. */
export function TermsPanel({ cycle, level, terms, onCreate }: TermsPanelProps): ReactElement {
  const suggestion = nextOrdinal(terms);
  const [name, setName] = useState("");
  const [ordinal, setOrdinal] = useState(suggestion);
  const [startsOn, setStartsOn] = useState("");
  const [endsOn, setEndsOn] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const canSubmit = name.trim() !== "" && ordinal >= 1 && startsOn !== "" && endsOn !== "" && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ cycleId: cycle.id, levelId: level.id, name: name.trim(), ordinal, startsOn, endsOn });
      setName("");
      setOrdinal(ordinal + 1);
      setStartsOn("");
      setEndsOn("");
    } catch {
      setError("No se pudo crear el periodo. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title={`Periodos de ${level.name}`} subtitle={`Periodos del nivel en ${cycle.name}.`}>
      {terms.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 6 }}>
          {terms.map((t) => (
            <li key={t.id} style={{ padding: "8px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", fontSize: 14 }}>
              {t.ordinal}. {t.name} · {t.startsOn} → {t.endsOn}
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay periodos para este nivel en el ciclo.</EmptyHint>
      )}

      <form aria-label="Agregar periodo" onSubmit={submit} style={{ display: "grid", gap: 12 }}>
        <div style={{ display: "grid", gridTemplateColumns: "1.4fr .6fr", gap: 12 }}>
          <TextField label="Nombre del periodo" value={name} onChange={setName} placeholder="T1" />
          <NumberField label="Orden" value={ordinal} onChange={setOrdinal} min={1} />
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr auto", gap: 12, alignItems: "end" }}>
          <DateField label="Inicio del periodo" value={startsOn} onChange={setStartsOn} />
          <DateField label="Fin del periodo" value={endsOn} onChange={setEndsOn} />
          <Btn type="submit" icon="plus" disabled={!canSubmit}>
            Crear periodo
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

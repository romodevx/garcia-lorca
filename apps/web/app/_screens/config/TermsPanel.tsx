"use client";

import { useEffect, useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
import type { CreateTermInput, Cycle, Level, Term } from "../../config/api";
import { DateField, NumberField, TextField } from "../../_design/form";
import { EmptyHint, Section } from "./parts";

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

/**
 * The periods a level is graded in — trimestres, bimestres — for one cycle.
 *
 * The list is what the panel shows; adding one opens the drawer. The order and
 * the dates come suggested, because a school setting up a year enters these
 * five times in a row and every extra keystroke is paid five times.
 */
export function TermsPanel({ cycle, level, terms, onCreate }: TermsPanelProps): ReactElement {
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [ordinal, setOrdinal] = useState(String(nextOrdinal(terms)));
  const [startsOn, setStartsOn] = useState("");
  const [endsOn, setEndsOn] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  useEffect(() => { setOrdinal(String(nextOrdinal(terms))); }, [terms]);

  const submit = async (): Promise<void> => {
    if (name.trim() === "" || startsOn === "" || endsOn === "") {
      setError("Un periodo necesita nombre y sus dos fechas.");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ cycleId: cycle.id, levelId: level.id, name: name.trim(), ordinal: Number(ordinal), startsOn, endsOn });
      setName("");
      setStartsOn("");
      setEndsOn("");
      setCreating(false);
    } catch {
      setError("No se pudo crear el periodo. Revisa las fechas y el orden.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title={`Periodos de ${level.name}`} subtitle={`En el ciclo ${cycle.name}.`}>
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

      <Btn icon="plus" onClick={() => { setError(undefined); setCreating(true); }}>Nuevo periodo</Btn>

      {creating && (
        <SlideOver
          title={`Nuevo periodo de ${level.name}`}
          subtitle={`Ciclo ${cycle.name}`}
          icon="calendar"
          submitLabel="Crear periodo"
          pending={pending}
          error={error}
          onClose={() => { setCreating(false); setError(undefined); }}
          onSubmit={() => void submit()}
        >
          <div style={{ display: "grid", gridTemplateColumns: "1.4fr .6fr", gap: 14 }}>
            <TextField label="Nombre del periodo" value={name} onChange={setName} uppercase required placeholder="T1" />
            <NumberField label="Orden" value={ordinal} onChange={setOrdinal} required min={1} />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
            <DateField label="Inicio del periodo" value={startsOn} onChange={setStartsOn} required />
            <DateField label="Fin del periodo" value={endsOn} onChange={setEndsOn} required />
          </div>
        </SlideOver>
      )}
    </Section>
  );
}

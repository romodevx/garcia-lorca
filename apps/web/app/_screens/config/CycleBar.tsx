"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { Cycle, CreateCycleInput } from "../../config/api";
import { DateField, EmptyHint, ErrorAlert, Section, SelectChip, TextField } from "./parts";

export interface CycleBarProps {
  cycles: readonly Cycle[];
  selectedCycleId: string | undefined;
  onSelect: (id: string) => void;
  onCreate: (input: CreateCycleInput) => Promise<void>;
}

const STATUS_LABEL: Record<string, string> = {
  draft: "Borrador",
  active: "Activo",
  closed: "Cerrado",
};

/** Pick an existing cycle or create one (status omitted → server defaults draft). */
export function CycleBar({ cycles, selectedCycleId, onSelect, onCreate }: CycleBarProps): ReactElement {
  const [name, setName] = useState("");
  const [startsOn, setStartsOn] = useState("");
  const [endsOn, setEndsOn] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

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
    } catch {
      setError("No se pudo crear el ciclo. Intenta de nuevo.");
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

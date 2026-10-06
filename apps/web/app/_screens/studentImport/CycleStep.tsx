"use client";

import { useState, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DateField, TextField, useRequired } from "../../_design/form";
import type { ImportCycle } from "./useImportCycle";
import { alertStyle } from "./parts";

/** The month (1–12) from which "this school year" means the one starting this August. */
const SCHOOL_YEAR_TURN = 7;
const REQUIRED = ["name", "startsOn", "endsOn"] as const;

/** "2026–2027", Aug 1 → Jul 31: the school year a migration most likely fills. */
export function suggestedCycle(today: string): { name: string; startsOn: string; endsOn: string } {
  const [year = 0, month = 1] = today.split("-").map(Number);
  const start = month >= SCHOOL_YEAR_TURN ? year : year - 1;
  return { name: `${start}–${start + 1}`, startsOn: `${start}-08-01`, endsOn: `${start + 1}-07-31` };
}

/** Step 0 (O9): without an active cycle the pupils have nowhere to enter. */
export function CycleStep({ cycle, today }: { cycle: ImportCycle; today: string }): ReactElement {
  const suggested = suggestedCycle(today);
  const [name, setName] = useState(suggested.name);
  const [startsOn, setStartsOn] = useState(suggested.startsOn);
  const [endsOn, setEndsOn] = useState(suggested.endsOn);
  const required = useRequired(REQUIRED);
  const dormant = cycle.dormant;

  const submit = (e: FormEvent): void => {
    e.preventDefault();
    if (!required.check({ name, startsOn, endsOn })) return;
    void cycle.createAndActivate({ name: name.trim(), startsOn, endsOn });
  };

  return (
    <div style={{ display: "grid", gap: 16 }}>
      <p style={{ fontSize: 15.5, color: "var(--text)", margin: 0 }}>Para migrar alumnos primero crea el ciclo escolar al que van a entrar</p>
      {cycle.error !== undefined && <p role="alert" style={alertStyle}>{cycle.error}</p>}
      {dormant !== undefined ? (
        <div style={{ display: "grid", gap: 10, justifyItems: "start" }}>
          <p style={{ fontSize: 14.5, color: "var(--text-muted)", margin: 0 }}>{`El ciclo ${dormant.name} ya existe, pero no está activo.`}</p>
          <Btn icon="check" onClick={() => void cycle.activate(dormant.id)}>{cycle.pending ? "Activando…" : `Activar «${dormant.name}»`}</Btn>
        </div>
      ) : (
        <form onSubmit={submit} noValidate style={{ display: "grid", gap: 14, maxWidth: 520 }}>
          <TextField label="Nombre del ciclo" value={name} onChange={setName} required error={required.errors.name} onClearError={() => required.clear("name")} />
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: 14 }}>
            <DateField label="Inicia" value={startsOn} onChange={setStartsOn} required error={required.errors.startsOn} onClearError={() => required.clear("startsOn")} />
            <DateField label="Termina" value={endsOn} onChange={setEndsOn} required error={required.errors.endsOn} onClearError={() => required.clear("endsOn")} />
          </div>
          <div><Btn type="submit" icon="check">{cycle.pending ? "Creando…" : "Crear y activar ciclo"}</Btn></div>
        </form>
      )}
    </div>
  );
}

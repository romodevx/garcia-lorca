"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Toast } from "../../_design/Toast";
import type { CopyTermMonthsInput, CreateTermInput, Cycle, Level, Term } from "../../config/api";
import { cycleMonths, trimesterInputs } from "../../config/termMonths";
import { CopyMonthsInline } from "./CopyMonthsInline";
import { NewTermRow } from "./NewTermRow";
import { EmptyHint, ErrorAlert, Section } from "./parts";
import { TermMonthsEditor } from "./TermMonthsEditor";
import { useTermMonths, type SaveTermMonths } from "./useTermMonths";

export interface TermsPanelProps {
  cycle: Cycle;
  level: Level;
  /** Every level of the school: the others can receive this one's months. */
  levels: readonly Level[];
  terms: readonly Term[];
  onCreate: (input: CreateTermInput) => Promise<void>;
  onSetMonths: SaveTermMonths;
  onCopyMonths: (input: CopyTermMonthsInput) => Promise<Term[]>;
}

const TOAST_MS = 4000;
const TRIMESTERS_FAILED = "No se pudieron crear los trimestres. Revisa las fechas del ciclo.";

function copiedMessage(count: number): string {
  return count === 1 ? "Se copiaron los meses a 1 nivel" : `Se copiaron los meses a ${count} niveles`;
}

/**
 * Los periodos en que se califica un nivel —trimestres, bimestres— de un ciclo,
 * y qué meses cuenta cada uno. Los meses NO son fijos: cada colegio los reparte
 * por ciclo y nivel, y de ahí sale el promedio de cada trimestre en la boleta.
 */
export function TermsPanel({ cycle, level, levels, terms, onCreate, onSetMonths, onCopyMonths }: TermsPanelProps): ReactElement {
  const months = useMemo(() => cycleMonths(cycle.startsOn, cycle.endsOn), [cycle.startsOn, cycle.endsOn]);
  const { assignment, error, tap } = useTermMonths(terms, onSetMonths);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const [createError, setCreateError] = useState<string | undefined>(undefined);
  const sorted = useMemo(() => [...terms].sort((a, b) => a.ordinal - b.ordinal), [terms]);

  useEffect(() => {
    if (toast === undefined) return;
    const timer = setTimeout(() => setToast(undefined), TOAST_MS);
    return () => clearTimeout(timer);
  }, [toast]);

  const createTrimesters = async (): Promise<void> => {
    setCreateError(undefined);
    try {
      // One after the other: the ordinals must arrive in order.
      for (const input of trimesterInputs(cycle, level.id)) await onCreate(input);
    } catch {
      setCreateError(TRIMESTERS_FAILED);
    }
  };

  const copy = async (toLevelIds: string[]): Promise<void> => {
    await onCopyMonths({ cycleId: cycle.id, fromLevelId: level.id, toLevelIds });
    setToast(copiedMessage(toLevelIds.length));
  };

  return (
    <Section title={`Periodos de ${level.name}`} subtitle={`En el ciclo ${cycle.name}. Toca un mes para sumarlo al periodo.`}>
      {sorted.length > 0 ? (
        <>
          <TermMonthsEditor terms={sorted} months={months} assignment={assignment} error={error} onTap={tap} />
          <div style={{ marginBottom: 16 }}>
            <CopyMonthsInline targets={levels.filter((l) => l.id !== level.id)} onCopy={copy} />
          </div>
        </>
      ) : (
        <div style={{ display: "grid", gap: 10, justifyItems: "start", marginBottom: 16 }}>
          <EmptyHint>Aún no hay periodos para este nivel en el ciclo.</EmptyHint>
          <Btn icon="plus" onClick={() => void createTrimesters()} style={{ height: 44 }}>Crear trimestres</Btn>
          {createError !== undefined && <ErrorAlert message={createError} />}
        </div>
      )}

      <NewTermRow cycle={cycle} level={level} terms={terms} onCreate={onCreate} />
      {toast !== undefined && <Toast>{toast}</Toast>}
    </Section>
  );
}

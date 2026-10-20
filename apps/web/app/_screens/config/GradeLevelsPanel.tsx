"use client";

import { type ReactElement } from "react";

import { InlineCreateRow } from "../../_design/InlineCreateRow";
import type { CreateGradeLevelInput, GradeLevel, Level } from "../../config/api";
import { CorrectableName, type NameCorrections } from "./CorrectableName";
import { useGradeCreateRow } from "./createRows";
import { DeletionStatus, useDeletionFeedback } from "./useDeletionFeedback";
import { EmptyHint, ErrorAlert, Section, SelectChip } from "./parts";
import { useSetupPresentation } from "./SetupPresentation";

export interface GradeLevelsPanelProps {
  level: Level;
  gradeLevels: readonly GradeLevel[];
  selectedGradeLevelId: string | undefined;
  onSelect: (id: string) => void;
  onCreate: (input: CreateGradeLevelInput) => Promise<void>;
  /** config:write: rename / delete beside each grade (T8). Absent = read-only chips. */
  corrections?: NameCorrections;
}

/**
 * Los grados de un nivel — 1° primaria, 2° primaria…
 *
 * Elegir uno y crear otro son actos distintos, y ninguno de los dos merecía un
 * cajón: una escuela captura los seis grados de primaria de corrido, y cada
 * cajón cobraba el viaje completo de abrir, mirar a otro lado y volver.
 *
 * El ORDEN sigue siendo un campo, con el siguiente ya escrito. Quitarlo habría
 * hecho la captura un clic más corta a cambio de volver imposible intercalar un
 * grado, que es un caso real y no tiene otra pantalla donde arreglarse.
 */
export function GradeLevelsPanel({
  level,
  gradeLevels,
  selectedGradeLevelId,
  onSelect,
  onCreate,
  corrections,
}: GradeLevelsPanelProps): ReactElement {
  const row = useGradeCreateRow(level, gradeLevels, onCreate);
  const feedback = useDeletionFeedback();
  // Inside a setup step a grade chip is its name alone: "1. 1°" said the order twice (§1.8 #12).
  const { inSetup } = useSetupPresentation();

  return (
    <Section title={`Grados de ${level.name}`} subtitle={inSetup ? "Elige uno para ver sus grupos, o agrega otro." : "Elige uno para sus materias y grupos, o agrega otro."}>
      {gradeLevels.length > 0 && corrections !== undefined && (
        <ul aria-label={`Grados de ${level.name}`} style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "start", listStyle: "none", padding: 0, margin: "0 0 16px" }}>
          {gradeLevels.map((g) => (
            <CorrectableName
              key={g.id}
              name={g.name}
              noun="grado"
              onRename={(name) => corrections.rename(g.id, name)}
              onDelete={() => corrections.remove(g.id)}
              onDeleted={(next) => feedback.deleted(`Grado ${g.name} borrado`, next)}
              deleteDescription={`Se borra el grado ${g.name} de ${level.name}. El nivel y sus otros grados no cambian.`}
            >
              <SelectChip selected={g.id === selectedGradeLevelId} onClick={() => onSelect(g.id)}>
                {inSetup ? g.name : <>{g.ordinal}. {g.name}</>}
              </SelectChip>
            </CorrectableName>
          ))}
        </ul>
      )}
      {gradeLevels.length > 0 && corrections === undefined && (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {gradeLevels.map((g) => (
            <SelectChip key={g.id} selected={g.id === selectedGradeLevelId} onClick={() => onSelect(g.id)}>
              {inSetup ? g.name : <>{g.ordinal}. {g.name}</>}
            </SelectChip>
          ))}
        </div>
      )}
      {gradeLevels.length === 0 && (
        <EmptyHint>Aún no hay grados en este nivel.</EmptyHint>
      )}

      <DeletionStatus message={feedback.message} />
      <div ref={feedback.fallbackRef} style={{ border: "1px solid var(--border)", borderRadius: "var(--r-md)", overflow: "hidden" }}>
        <InlineCreateRow addLabel="Nuevo grado" rules={row.rules} defaults={row.defaults} columns={row.columns} onCreate={row.onCreate} />
      </div>

      {row.error !== undefined && <ErrorAlert message={row.error} />}
    </Section>
  );
}

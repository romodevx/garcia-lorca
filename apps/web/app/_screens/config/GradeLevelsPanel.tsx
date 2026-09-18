"use client";

import { useMemo, useState, type ReactElement } from "react";

import { InlineCreateRow } from "../../_design/InlineCreateRow";
import type { CreateGradeLevelInput, GradeLevel, Level } from "../../config/api";
import { EmptyHint, ErrorAlert, Section, SelectChip } from "./parts";

export interface GradeLevelsPanelProps {
  level: Level;
  gradeLevels: readonly GradeLevel[];
  selectedGradeLevelId: string | undefined;
  onSelect: (id: string) => void;
  onCreate: (input: CreateGradeLevelInput) => Promise<void>;
}

/** El orden que sigue, para no obligar a nadie a contar la lista. */
function nextOrdinal(gradeLevels: readonly GradeLevel[]): number {
  return gradeLevels.reduce((max, g) => Math.max(max, g.ordinal), 0) + 1;
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
}: GradeLevelsPanelProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);

  // Identidad estable: `InlineCreateRow` la observa para poner al día la celda
  // que nadie tocó, y un objeto nuevo en cada render la reescribiría siempre.
  const defaults = useMemo(
    () => ({ ordinal: String(nextOrdinal(gradeLevels)) }),
    [gradeLevels],
  );

  return (
    <Section title={`Grados de ${level.name}`} subtitle="Elige uno para sus materias y grupos, o agrega otro.">
      {gradeLevels.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {gradeLevels.map((g) => (
            <SelectChip key={g.id} selected={g.id === selectedGradeLevelId} onClick={() => onSelect(g.id)}>
              {g.ordinal}. {g.name}
            </SelectChip>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay grados en este nivel.</EmptyHint>
      )}

      <div style={{ border: "1px solid var(--border)", borderRadius: "var(--r-md)", overflow: "hidden" }}>
        <InlineCreateRow
          addLabel="Nuevo grado"
          defaults={defaults}
          columns={[
            {
              key: "name",
              label: "Nombre del grado",
              kind: "text",
              uppercase: true,
              required: true,
              placeholder: "1° PRIMARIA",
            },
            { key: "ordinal", label: "Orden", kind: "number", required: true, width: 110 },
          ]}
          onCreate={async (values) => {
            const ordinal = Number(values.ordinal);
            if (!Number.isFinite(ordinal) || ordinal < 1) {
              // Number("") es 0, y un 0 que nadie escribió es un dato falso.
              setError("El orden del grado tiene que ser un número mayor que cero.");
              // También se relanza: para el renglón, "no pasó" es no pasó, sea
              // por la red o por el dato.
              throw new Error("El orden del grado tiene que ser un número mayor que cero.");
            }
            setError(undefined);
            try {
              await onCreate({ levelId: level.id, name: (values.name ?? "").trim(), ordinal });
            } catch (cause) {
              setError("No se pudo crear el grado. Revisa que no exista ya con ese orden.");
              // Se RELANZA: el renglón necesita saber que no se guardó para
              // conservar lo escrito. Tragarse el error aquí dejaba el mensaje
              // hablando de un renglón que ya se había vaciado solo, y obligaba
              // a reescribir todo.
              throw cause;
            }
          }}
        />
      </div>

      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

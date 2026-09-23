"use client";

import { useMemo, useState, type ReactElement } from "react";

import { InlineCreateRow } from "../../_design/InlineCreateRow";
import type { CreateTermInput, Cycle, Level, Term } from "../../config/api";
import { EmptyHint, ErrorAlert, Section } from "./parts";

export interface TermsPanelProps {
  cycle: Cycle;
  level: Level;
  terms: readonly Term[];
  onCreate: (input: CreateTermInput) => Promise<void>;
}

/** El orden que sigue, para no obligar a nadie a contar la lista. */
function nextOrdinal(terms: readonly Term[]): number {
  return terms.reduce((max, t) => Math.max(max, t.ordinal), 0) + 1;
}

/**
 * Los periodos en que se califica un nivel —trimestres, bimestres— de un ciclo.
 *
 * Una escuela captura estos cinco seguidos al abrir el año, así que cada tecla
 * de más se paga cinco veces. El renglón deja los cinco en una sola sentada, con
 * el orden ya propuesto y el fin revisado contra el inicio al salir de la celda.
 */
export function TermsPanel({ cycle, level, terms, onCreate }: TermsPanelProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);

  const defaults = useMemo(() => ({ ordinal: String(nextOrdinal(terms)) }), [terms]);

  // La fecha imposible se revisa al salir de la celda, no al teclear: el dato ya
  // está escrito y la persona sigue parada ahí, que es cuando corregir es barato.
  const rules = useMemo(
    () => ({
      endsOn: (value: string, all: Readonly<Record<string, string>>): string | undefined => {
        const from = all.startsOn ?? "";
        return from !== "" && value < from ? "El fin del periodo es antes que su inicio." : undefined;
      },
    }),
    [],
  );

  return (
    <Section title={`Periodos de ${level.name}`} subtitle={`En el ciclo ${cycle.name}.`}>
      {terms.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 6 }}>
          {terms.map((t) => (
            <li
              key={t.id}
              style={{ padding: "8px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", fontSize: 14 }}
            >
              {t.ordinal}. {t.name} · {t.startsOn} → {t.endsOn}
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay periodos para este nivel en el ciclo.</EmptyHint>
      )}

      <div style={{ border: "1px solid var(--border)", borderRadius: "var(--r-md)", overflow: "hidden" }}>
        <InlineCreateRow
          addLabel="Nuevo periodo"
          defaults={defaults}
          rules={rules}
          columns={[
            {
              key: "name",
              label: "Nombre del periodo",
              kind: "text",
              uppercase: true,
              required: true,
              help: "Cómo se llama el periodo de evaluación. Es lo que verán los docentes al capturar.",
            },
            { key: "ordinal", label: "Orden", kind: "number", required: true, width: 100 },
            { key: "startsOn", label: "Inicio del periodo", kind: "date", required: true, width: 165 },
            { key: "endsOn", label: "Fin del periodo", kind: "date", required: true, width: 165 },
          ]}
          onCreate={async (values) => {
            const ordinal = Number(values.ordinal);
            if (!Number.isFinite(ordinal) || ordinal < 1) {
              // Number("") es 0, y un 0 que nadie escribió es un dato falso.
              setError("El orden del periodo tiene que ser un número mayor que cero.");
              // También se relanza: para el renglón, "no pasó" es no pasó, sea
              // por la red o por el dato.
              throw new Error("El orden del periodo tiene que ser un número mayor que cero.");
            }
            setError(undefined);
            try {
              await onCreate({
                cycleId: cycle.id,
                levelId: level.id,
                name: (values.name ?? "").trim(),
                ordinal,
                startsOn: values.startsOn ?? "",
                endsOn: values.endsOn ?? "",
              });
            } catch (cause) {
              setError("No se pudo crear el periodo. Revisa las fechas y el orden.");
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

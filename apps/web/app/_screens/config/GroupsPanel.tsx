"use client";

import { useMemo, useState, type ReactElement } from "react";

import { InlineCreateRow } from "../../_design/InlineCreateRow";
import type { CreateGroupInput, Cycle, GradeLevel, Group } from "../../config/api";
import { EmptyHint, ErrorAlert, Section } from "./parts";

export interface GroupsPanelProps {
  cycle: Cycle;
  gradeLevel: GradeLevel;
  groups: readonly Group[];
  onCreate: (input: CreateGroupInput) => Promise<void>;
}

const DEFAULT_CAPACITY = 30;

/** La letra que sigue: A, B, C… */
function nextLetter(groups: readonly Group[]): string {
  return String.fromCharCode("A".charCodeAt(0) + Math.min(groups.length, 25));
}

/**
 * Los salones de un grado en un ciclo: A, B, C…
 *
 * Es el alta más repetitiva de todo el catálogo —una escuela abre veinte grupos
 * en una tarde y luego no vuelve hasta el agosto siguiente— y por eso es donde
 * el cajón costaba más: veinte viajes para escribir veinte letras.
 *
 * Las dos celdas llegan propuestas, así que abrir el renglón y dar Enter crea el
 * grupo. Pero siguen siendo campos: el CUPO decide a qué grupo entra un alumno
 * nuevo, y un salón más chico que los demás tiene que poder capturarse.
 */
export function GroupsPanel({ cycle, gradeLevel, groups, onCreate }: GroupsPanelProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);

  // Identidad estable: el renglón la observa para poner al día la celda que
  // nadie tocó, y un objeto nuevo en cada render la reescribiría siempre.
  const defaults = useMemo(
    () => ({ name: nextLetter(groups), capacity: String(DEFAULT_CAPACITY) }),
    [groups],
  );

  return (
    <Section title="Grupos" subtitle={`De ${gradeLevel.name} en ${cycle.name}.`}>
      {groups.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {groups.map((g) => (
            <span
              key={g.id}
              style={{ padding: "8px 14px", borderRadius: "var(--r-pill)", background: "var(--surface-2)", fontSize: 14, fontWeight: 600 }}
            >
              {g.name} · {g.capacity} lugares
            </span>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay grupos para este grado en el ciclo.</EmptyHint>
      )}

      <div style={{ border: "1px solid var(--border)", borderRadius: "var(--r-md)", overflow: "hidden" }}>
        <InlineCreateRow
          addLabel="Nuevo grupo"
          defaults={defaults}
          columns={[
            {
              key: "name",
              label: "Nombre del grupo",
              kind: "text",
              uppercase: true,
              required: true,
              placeholder: "A",
            },
            { key: "capacity", label: "Cupo", kind: "number", required: true, width: 120 },
          ]}
          onCreate={async (values) => {
            const capacity = Number(values.capacity);
            if (!Number.isFinite(capacity) || capacity < 1) {
              // Number("") es 0, y un cupo 0 que nadie escribió no acepta
              // alumnos: la asignación automática se salta ese grupo en silencio.
              setError("El cupo del grupo tiene que ser un número mayor que cero.");
              // También se relanza: para el renglón, "no pasó" es no pasó, sea
              // por la red o por el dato.
              throw new Error("El cupo del grupo tiene que ser un número mayor que cero.");
            }
            setError(undefined);
            try {
              await onCreate({
                cycleId: cycle.id,
                gradeLevelId: gradeLevel.id,
                name: (values.name ?? "").trim(),
                capacity,
              });
            } catch (cause) {
              setError("No se pudo crear el grupo. Revisa que no exista ya con ese nombre.");
              // Se RELANZA: el renglón necesita saber que no se guardó para
              // conservar lo escrito. Tragarse el error aquí dejaba el mensaje
              // hablando de un renglón que ya se había vaciado solo, y obligaba
              // a reescribir todo.
              throw cause;
            }
          }}
        />
      </div>

      <p style={{ fontSize: 13, color: "var(--text-faint)", marginTop: 10, lineHeight: 1.5 }}>
        El <strong>cupo</strong> decide a qué grupo entra un alumno nuevo: se llena A y el
        siguiente pasa a B, sin que nadie lo asigne a mano.
      </p>

      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

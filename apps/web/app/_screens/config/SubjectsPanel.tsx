"use client";

import { useState, type ReactElement } from "react";

import { InlineCreateChip } from "../../_design/InlineCreateChip";
import type { CreateSubjectInput, GradeLevel, Subject } from "../../config/api";
import { EmptyHint, ErrorAlert, Section } from "./parts";

export interface SubjectsPanelProps {
  gradeLevel: GradeLevel;
  subjects: readonly Subject[];
  onCreate: (input: CreateSubjectInput) => Promise<void>;
}

/**
 * Las materias que se dan en un grado.
 *
 * Un solo dato, y diez o doce seguidos. Es el caso donde un cajón era más caro
 * que la captura: abrir, escribir ocho letras, guardar, esperar el cierre, y
 * otra vez. Ahora el chip se queda abierto y cada Enter agrega la siguiente.
 */
export function SubjectsPanel({ gradeLevel, subjects, onCreate }: SubjectsPanelProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);

  return (
    <Section title="Materias" subtitle={`De ${gradeLevel.name}.`}>
      {subjects.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 6 }}>
          {subjects.map((s) => (
            <li
              key={s.id}
              style={{ padding: "8px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", fontSize: 14 }}
            >
              {s.name}
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay materias en este grado.</EmptyHint>
      )}

      <InlineCreateChip
        label="Nueva materia"
        fieldLabel="Nombre de la materia"
        placeholder="MATEMÁTICAS"
        onCreate={async (name) => {
          setError(undefined);
          try {
            await onCreate({ gradeLevelId: gradeLevel.id, name });
          } catch (cause) {
            setError("No se pudo crear la materia. Revisa que no exista ya en este grado.");
            // El chip conserva lo escrito sólo si se entera de que no se guardó.
            throw cause;
          }
        }}
      />

      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

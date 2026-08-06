"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateSubjectInput, GradeLevel, Subject } from "../../config/api";
import { EmptyHint, ErrorAlert, Section, TextField } from "./parts";

export interface SubjectsPanelProps {
  gradeLevel: GradeLevel;
  subjects: readonly Subject[];
  onCreate: (input: CreateSubjectInput) => Promise<void>;
}

/** Subjects of the selected grade level. */
export function SubjectsPanel({ gradeLevel, subjects, onCreate }: SubjectsPanelProps): ReactElement {
  const [name, setName] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const canSubmit = name.trim() !== "" && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ gradeLevelId: gradeLevel.id, name: name.trim() });
      setName("");
    } catch {
      setError("No se pudo crear la materia. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Materias" subtitle={`Materias de ${gradeLevel.name}.`}>
      {subjects.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 6 }}>
          {subjects.map((s) => (
            <li key={s.id} style={{ padding: "8px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", fontSize: 14 }}>
              {s.name}
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay materias en este grado.</EmptyHint>
      )}

      <form aria-label="Agregar materia" onSubmit={submit}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr auto", gap: 12, alignItems: "end" }}>
          <TextField label="Nombre de la materia" value={name} onChange={setName} placeholder="Matemáticas" />
          <Btn type="submit" icon="plus" disabled={!canSubmit}>
            Crear materia
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

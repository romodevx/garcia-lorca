"use client";

import { useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
import type { CreateSubjectInput, GradeLevel, Subject } from "../../config/api";
import { EmptyHint, Section, TextField } from "./parts";

export interface SubjectsPanelProps {
  gradeLevel: GradeLevel;
  subjects: readonly Subject[];
  onCreate: (input: CreateSubjectInput) => Promise<void>;
}

/** Subjects of the selected grade level. */
/** The subjects taught in a grade. List here, create in the drawer. */
export function SubjectsPanel({ gradeLevel, subjects, onCreate }: SubjectsPanelProps): ReactElement {
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const submit = async (): Promise<void> => {
    if (name.trim() === "") {
      setError("Escribe el nombre de la materia.");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ gradeLevelId: gradeLevel.id, name: name.trim() });
      setName("");
      setCreating(false);
    } catch {
      setError("No se pudo crear la materia. Revisa que no exista ya en este grado.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Materias" subtitle={`De ${gradeLevel.name}.`}>
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

      <Btn icon="plus" onClick={() => { setError(undefined); setCreating(true); }}>Nueva materia</Btn>

      {creating && (
        <SlideOver
          title={`Nueva materia en ${gradeLevel.name}`}
          icon="grades"
          submitLabel="Crear materia"
          pending={pending}
          error={error}
          onClose={() => { setCreating(false); setError(undefined); }}
          onSubmit={() => void submit()}
        >
          <TextField label="Nombre de la materia" value={name} onChange={setName} placeholder="Matemáticas" />
        </SlideOver>
      )}
    </Section>
  );
}

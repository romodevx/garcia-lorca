"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateStudentInput } from "../../students/api";
import { SelectField, TextField, type SelectOption } from "../../_design/form";
import { ErrorAlert, Section } from "../config/parts";

// The admission level is a fixed enum on the students service; the gateway maps
// these exact canonical (lowercase) values to the wire enum, so a free-text nivel
// would be rejected. Keep the values in lockstep with levelNames in
// services/gateway/internal/client/students.go.
const LEVEL_OPTIONS: readonly SelectOption[] = [
  { value: "maternal", label: "Maternal" },
  { value: "kinder", label: "Kínder" },
  { value: "primaria", label: "Primaria" },
  { value: "secundaria", label: "Secundaria" },
  { value: "preparatoria", label: "Preparatoria" },
];

export interface StudentCreatePanelProps {
  onCreate: (input: CreateStudentInput) => Promise<void>;
}

/**
 * Register a new applicant. Admission begins at "pending" (the students service
 * owns the state machine); once created the student becomes enrollable in the
 * panel beside this one. The gateway (behind module_students + students:create)
 * is the security boundary — this screen is UX only.
 */
export function StudentCreatePanel({ onCreate }: StudentCreatePanelProps): ReactElement {
  const [curp, setCurp] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [level, setLevel] = useState("");
  const [gradeGroup, setGradeGroup] = useState("");
  const [guardianName, setGuardianName] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const canSubmit =
    curp.trim() !== "" &&
    firstName.trim() !== "" &&
    lastName.trim() !== "" &&
    level.trim() !== "" &&
    gradeGroup.trim() !== "" &&
    guardianName.trim() !== "" &&
    !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({
        curp: curp.trim(),
        firstName: firstName.trim(),
        lastName: lastName.trim(),
        level: level.trim(),
        gradeGroup: gradeGroup.trim(),
        guardianName: guardianName.trim(),
      });
      setCurp("");
      setFirstName("");
      setLastName("");
      setLevel("");
      setGradeGroup("");
      setGuardianName("");
    } catch {
      setError("No se pudo registrar al alumno. Verifica el CURP (único) e intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Registrar alumno" subtitle="Alta de aspirante. La admisión inicia en «pendiente».">
      <form aria-label="Registrar alumno" onSubmit={submit}>
        <div style={{ display: "grid", gap: 12, marginBottom: 12 }}>
          <TextField label="CURP" value={curp} onChange={setCurp} uppercase required help="Los 18 caracteres de la CURP del alumno, tal como vienen en el acta. Es lo que impide que el mismo alumno se registre dos veces." />
          <TextField label="Nombre" value={firstName} onChange={setFirstName} uppercase required help="Sólo los nombres de pila, sin apellidos." />
          <TextField label="Apellidos" value={lastName} onChange={setLastName} uppercase required help="Los dos apellidos: primero el paterno, luego el materno." />
          <SelectField label="Nivel" value={level} options={LEVEL_OPTIONS} onChange={setLevel} required help="El nivel educativo donde va a cursar: preescolar, primaria o secundaria." />
          <TextField label="Grado y grupo" value={gradeGroup} onChange={setGradeGroup} uppercase help="El grado y la letra del salón, juntos." />
          <TextField label="Tutor" value={guardianName} onChange={setGuardianName} uppercase help="Quién responde por el alumno ante el colegio: nombre completo de la madre, el padre o el tutor legal." />
        </div>
        <Btn type="submit" icon="plus" disabled={!canSubmit}>
          Registrar alumno
        </Btn>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

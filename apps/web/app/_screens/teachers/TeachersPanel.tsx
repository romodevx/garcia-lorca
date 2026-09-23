"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateTeacherInput, Teacher } from "../../teachers/api";
import { EmailField, TextField } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";

export interface TeachersPanelProps {
  teachers: readonly Teacher[];
  onCreate: (input: CreateTeacherInput) => Promise<void>;
}

/**
 * Teachers catalog: person records (NOT login accounts). `userId` is reserved
 * for a future IAM link and intentionally not collected here. Dumb panel — data
 * + an async `onCreate` come from the Catalog orchestrator.
 */
export function TeachersPanel({ teachers, onCreate }: TeachersPanelProps): ReactElement {
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [email, setEmail] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const canSubmit = firstName.trim() !== "" && lastName.trim() !== "" && email.trim() !== "" && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ firstName: firstName.trim(), lastName: lastName.trim(), email: email.trim() });
      setFirstName("");
      setLastName("");
      setEmail("");
    } catch {
      setError("No se pudo crear al profesor. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Profesores" subtitle="Personal docente del plantel.">
      {teachers.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 6 }}>
          {teachers.map((t) => (
            <li
              key={t.id}
              style={{ padding: "8px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", fontSize: 14 }}
            >
              <span style={{ fontWeight: 600 }}>
                {t.firstName} {t.lastName}
              </span>{" "}
              <span style={{ color: "var(--text-muted)" }}>· {t.email}</span>
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay profesores registrados.</EmptyHint>
      )}

      <form aria-label="Agregar profesor" onSubmit={submit}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, marginBottom: 12 }}>
          <TextField label="Nombre" value={firstName} onChange={setFirstName} uppercase required help="Sólo los nombres de pila del docente, sin apellidos." />
          <TextField label="Apellidos" value={lastName} onChange={setLastName} uppercase required help="Los dos apellidos: primero el paterno, luego el materno." />
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "1fr auto", gap: 12, alignItems: "end" }}>
          <EmailField label="Correo" value={email} onChange={setEmail} help="El correo donde el docente recibirá su acceso. Puede quedar vacío y agregarse después." />
          <Btn type="submit" icon="plus" disabled={!canSubmit}>
            Crear profesor
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

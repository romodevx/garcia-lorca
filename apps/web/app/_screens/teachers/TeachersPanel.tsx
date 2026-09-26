"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateTeacherInput, Teacher } from "../../teachers/api";
import { EmailField, TextField, useEmailShapeOnSave, useRequired } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";

const REQUIRED = ["firstName", "lastName", "email"] as const;

export interface TeachersPanelProps {
  teachers: readonly Teacher[];
  onCreate: (input: CreateTeacherInput) => Promise<void>;
  /**
   * Opens the teacher's carga académica page. Offered to everyone who sees the
   * roster (teachers:read): the page itself hides its write controls from
   * those without teachers:write. Absent when the academic catalogue is
   * closed to this account: the page could not name a single class.
   */
  onOpenLoad?: (teacherId: string) => void;
}

/**
 * Teachers catalog: person records (NOT login accounts). `userId` is reserved
 * for a future IAM link and intentionally not collected here. Dumb panel — data
 * + an async `onCreate` come from the Catalog orchestrator.
 */
export function TeachersPanel({ teachers, onCreate, onOpenLoad }: TeachersPanelProps): ReactElement {
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [email, setEmail] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  // Enter guarda sin salir del campo, así que la forma del correo también se
  // revisa aquí, al guardar, y no sólo al salir de él.
  const emailShape = useEmailShapeOnSave();
  // The server requires all three. The button is never disabled: saving says
  // what is missing, on the field (ui-forms-and-modals rules).
  const required = useRequired(REQUIRED);

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (pending) return;
    if (!required.check({ firstName, lastName, email })) return;
    if (!emailShape.check(email)) return;
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
              style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 8, padding: "6px 6px 6px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", fontSize: 14 }}
            >
              <span style={{ minWidth: 0, overflowWrap: "anywhere" }}>
                <span style={{ fontWeight: 600 }}>
                  {t.firstName} {t.lastName}
                </span>{" "}
                <span style={{ color: "var(--text-muted)" }}>· {t.email}</span>
              </span>
              {onOpenLoad !== undefined && (
              <button
                type="button"
                onClick={() => onOpenLoad(t.id)}
                aria-label={`Carga académica de ${t.firstName} ${t.lastName}`}
                style={{ minHeight: 44, padding: "0 14px", borderRadius: "var(--r-pill)", border: "1.5px solid var(--forest)", color: "var(--forest)", fontWeight: 600, fontSize: 13.5, cursor: "pointer" }}
              >
                Carga académica
              </button>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay profesores registrados.</EmptyHint>
      )}

      <form aria-label="Agregar profesor" onSubmit={submit}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, marginBottom: 12 }}>
          <TextField label="Nombre" value={firstName} onChange={setFirstName} uppercase required error={required.errors.firstName} onClearError={() => required.clear("firstName")} help="Sólo los nombres de pila del docente, sin apellidos." />
          <TextField label="Apellidos" value={lastName} onChange={setLastName} uppercase required error={required.errors.lastName} onClearError={() => required.clear("lastName")} help="Los dos apellidos: primero el paterno, luego el materno." />
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "1fr auto", gap: 12, alignItems: "end" }}>
          <EmailField
            label="Correo"
            value={email}
            onChange={(v) => {
              setEmail(v);
              emailShape.forgive(v);
            }}
            required
            onClearError={() => required.clear("email")}
            help="El correo donde el docente recibirá su acceso."
            error={required.errors.email ?? emailShape.error}
          />
          <Btn type="submit" icon="plus">
            Crear profesor
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

"use client";

import { useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { Toast } from "../../_design/Toast";
import type { UserStatus } from "../../admin/api";
import type { CreateTeacherInput, Teacher } from "../../teachers/api";
import { describeCreateTeacherError } from "../../teachers/writeErrors";
import { EmailField, TextField, useEmailShapeOnSave, useRequired } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { TeacherRow } from "./TeacherRow";
import { useTeacherAccess, type GrantAccessCall } from "./useTeacherAccess";

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
  /**
   * Gives the teacher a login (or sends its invitation again). Absent unless
   * the principal holds teachers:write AND users:write.
   */
  onGrantAccess?: GrantAccessCall;
  /**
   * user id → account status, when the principal may list users (users:read).
   * Without it a linked teacher's login is shown as "Vinculado" and offers
   * nothing: its state is not known, so no claim is made about it.
   */
  accountStatuses?: ReadonlyMap<string, UserStatus>;
  /** Register a teacher's absence (T19). Absent without schedule:write or with Horarios off. */
  onRegisterAbsence?: (teacherId: string) => void;
  /** Start with the cursor on the new teacher's name ("Agregar profesor" shortcut). */
  focusNew?: boolean;
}

/**
 * Teachers catalog: person records. The login is not collected here: "Dar
 * acceso al sistema" on the row creates it and fills `userId`. Dumb panel —
 * data and the async writes come from the screen.
 */
export function TeachersPanel({ teachers, onCreate, onOpenLoad, onGrantAccess, accountStatuses, onRegisterAbsence, focusNew = false }: TeachersPanelProps): ReactElement {
  const form = useRef<HTMLFormElement>(null);
  // The fields are shared design-system inputs without autoFocus; the form's
  // first input is the name. Scrolling it into view is the browser's focus().
  useEffect(() => {
    if (focusNew) form.current?.querySelector("input")?.focus();
  }, [focusNew]);
  const access = useTeacherAccess(onGrantAccess, accountStatuses);
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
    } catch (cause) {
      setError(describeCreateTeacherError(cause));
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Profesores" subtitle="Personal docente del plantel.">
      {teachers.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 6 }}>
          {teachers.map((t) => (
            <TeacherRow
              key={t.id}
              teacher={t}
              access={access.stateOf(t)}
              granting={access.isGranting(t.id)}
              error={access.errorOf(t.id)}
              inviteUrl={access.inviteUrlOf(t.id)}
              sendError={access.sendErrorOf(t.id)}
              confirmation={access.confirmationOf(t.id)}
              onConfirmReuse={() => void access.confirmReuse(t)}
              onCancelReuse={() => access.cancelReuse(t.id)}
              onGrantAccess={onGrantAccess === undefined ? undefined : () => void access.grant(t)}
              onOpenLoad={onOpenLoad === undefined ? undefined : () => onOpenLoad(t.id)}
              onRegisterAbsence={onRegisterAbsence === undefined ? undefined : () => onRegisterAbsence(t.id)}
            />
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay profesores registrados.</EmptyHint>
      )}

      <form ref={form} aria-label="Agregar profesor" onSubmit={submit}>
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
          <Btn type="submit" icon="plus" busy={pending}>
            {pending ? "Guardando…" : "Crear profesor"}
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
      {access.toast !== undefined && <Toast>{access.toast}</Toast>}
    </Section>
  );
}

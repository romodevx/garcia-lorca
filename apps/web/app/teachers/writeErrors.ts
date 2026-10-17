import { ConfigError } from "../config/api";

const BAD_REQUEST = 400;
const FORBIDDEN = 403;
const CONFLICT = 409;

const NO_PERMISSION = "No tienes permiso para cambiar la plantilla docente.";
const TEACHER_EMAIL_TAKEN = "Ya existe un docente con ese correo.";
const SLOT_TAKEN = "Esa asignatura ya tiene docente en ese grupo.";

/** What one write is, and the words for the refusal only it can get. */
interface WriteKind {
  failedAction: string;
  conflict: string;
}

const CREATE_TEACHER: WriteKind = { failedAction: "No se pudo crear al profesor.", conflict: TEACHER_EMAIL_TAKEN };
const ASSIGN_TEACHER: WriteKind = { failedAction: "No se pudo asignar al profesor.", conflict: SLOT_TAKEN };

/**
 * A refused write, in words the person can act on (ISSUE-012). The server's
 * 409 reasons are English sentences, so the known ones are said in Spanish;
 * a 400 keeps the server's own words, because they name the field. Only a
 * request that never got an answer is blamed on the connection.
 */
function describe(cause: unknown, kind: WriteKind): string {
  if (!(cause instanceof ConfigError)) return `${kind.failedAction} Revisa tu conexión e intenta de nuevo.`;
  if (cause.status === CONFLICT) return kind.conflict;
  if (cause.status === FORBIDDEN) return NO_PERMISSION;
  if (cause.status === BAD_REQUEST && cause.detail !== "") return `${kind.failedAction} ${cause.detail}`;
  return `${kind.failedAction} Intenta de nuevo.`;
}

/** Why "Crear profesor" was refused. */
export function describeCreateTeacherError(cause: unknown): string {
  return describe(cause, CREATE_TEACHER);
}

/** Why "Asignar profesor" was refused. */
export function describeAssignTeacherError(cause: unknown): string {
  return describe(cause, ASSIGN_TEACHER);
}

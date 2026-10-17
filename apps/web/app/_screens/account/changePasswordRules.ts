import { HttpError } from "../../auth/httpError";

/** Mirrors the gateway's floor; iam stays the authority. */
export const MIN_PASSWORD_LENGTH = 8;

export type PasswordField = "current" | "next" | "confirm";

export type PasswordErrors = Partial<Record<PasswordField, string>>;

/** Field order = reading order: the first key with an error gets focus. */
export const FIELD_ORDER: readonly PasswordField[] = ["current", "next", "confirm"];

export interface PasswordDraft {
  current: string;
  next: string;
  confirm: string;
}

/** The client-side checks, run on submit only (UX; the server decides). */
export function validatePasswordDraft({ current, next, confirm }: PasswordDraft): PasswordErrors {
  const errors: PasswordErrors = {};
  if (current === "") errors.current = "Escribe tu contraseña actual.";
  if (next === "") errors.next = "Escribe la nueva contraseña.";
  else if (next.length < MIN_PASSWORD_LENGTH) errors.next = `Usa al menos ${MIN_PASSWORD_LENGTH} caracteres.`;
  if (confirm === "") errors.confirm = "Vuelve a escribir la nueva contraseña.";
  else if (next !== confirm) errors.confirm = "No coincide con la nueva contraseña.";
  return errors;
}

export interface ChangeFailure {
  field?: PasswordField;
  message: string;
}

const UNAUTHORIZED = 401;
const BAD_REQUEST = 400;

/**
 * The 400 bodies the gateway writes for people (services/gateway handler
 * auth.go). Anything else — iam's bare "Bad Request" — is never shown verbatim.
 */
const KNOWN_SERVER_MESSAGES: ReadonlySet<string> = new Set([
  `La nueva contraseña debe tener al menos ${MIN_PASSWORD_LENGTH} caracteres.`,
]);

/** Says why the change failed, in Spanish. */
export function describeChangeFailure(err: unknown): ChangeFailure {
  if (!(err instanceof HttpError)) {
    return { message: "No pudimos contactar al servidor. Revisa tu conexión e intenta de nuevo." };
  }
  if (err.status === UNAUTHORIZED) return { field: "current", message: "La contraseña actual no es correcta." };
  if (err.status === BAD_REQUEST) {
    if (err.serverText !== undefined && KNOWN_SERVER_MESSAGES.has(err.serverText)) return { message: err.serverText };
    return { message: `La nueva contraseña no cumple los requisitos: usa al menos ${MIN_PASSWORD_LENGTH} caracteres.` };
  }
  return { message: "No se pudo cambiar la contraseña. Intenta de nuevo en un momento." };
}

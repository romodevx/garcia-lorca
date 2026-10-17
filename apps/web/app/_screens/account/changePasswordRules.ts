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

/** Why the change failed: on a field, for the whole form, or the session is gone. */
export type ChangeFailure =
  | { kind: "field"; field: PasswordField; message: string }
  | { kind: "form"; message: string }
  | { kind: "sessionExpired" };

/** Why the session ended after (or while) changing the password. */
export type SessionEndReason = "passwordChanged" | "sessionExpired";

const UNAUTHORIZED = 401;
const BAD_REQUEST = 400;
const TOO_MANY_REQUESTS = 429;

/** The gateway's code for "the current password is not the caller's". */
const REAUTH_FAILED = "REAUTH_FAILED";

/**
 * The 400 bodies the gateway writes for people (services/gateway handler
 * auth.go). Anything else (iam's bare "Bad Request") is never shown verbatim.
 */
const KNOWN_SERVER_MESSAGES: ReadonlySet<string> = new Set([
  `La nueva contraseña debe tener al menos ${MIN_PASSWORD_LENGTH} caracteres.`,
]);

const form = (message: string): ChangeFailure => ({ kind: "form", message });

/**
 * Says why the change failed, in Spanish. A 401 is the current password ONLY
 * when the gateway says so (REAUTH_FAILED); without the code the session
 * itself is gone, and asking for the password again would not help.
 */
export function describeChangeFailure(err: unknown): ChangeFailure {
  if (!(err instanceof HttpError)) return form("No pudimos contactar al servidor. Revisa tu conexión e intenta de nuevo.");
  if (err.status === UNAUTHORIZED) {
    return err.errorCode === REAUTH_FAILED
      ? { kind: "field", field: "current", message: "La contraseña actual no es correcta." }
      : { kind: "sessionExpired" };
  }
  if (err.status === BAD_REQUEST) {
    if (err.serverText !== undefined && KNOWN_SERVER_MESSAGES.has(err.serverText)) return form(err.serverText);
    return form(`La nueva contraseña no cumple los requisitos: usa al menos ${MIN_PASSWORD_LENGTH} caracteres.`);
  }
  if (err.status === TOO_MANY_REQUESTS) return form("Demasiados intentos. Espera unos minutos e intenta de nuevo.");
  return form("No se pudo cambiar la contraseña. Intenta de nuevo en un momento.");
}

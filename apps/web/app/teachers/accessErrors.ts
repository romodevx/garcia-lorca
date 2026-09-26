import { ConfigError } from "../config/api";

const BAD_REQUEST = 400;
const FORBIDDEN = 403;
const NOT_FOUND = 404;
const TOO_MANY_REQUESTS = 429;

const FAILED = "No se pudo dar el acceso.";

/**
 * What a failed "Dar acceso al sistema" tells the office. A 400 carries the
 * server's own reason (a teacher without email), which is already the fix;
 * the other known statuses get plain Spanish; a failure with no answer is
 * blamed on the connection.
 */
export function describeAccessError(cause: unknown): string {
  if (!(cause instanceof ConfigError)) return `${FAILED} Revisa tu conexión e intenta de nuevo.`;
  if (cause.status === BAD_REQUEST && cause.detail !== "") return cause.detail;
  if (cause.status === FORBIDDEN) return "No tienes permiso para dar acceso a docentes.";
  if (cause.status === NOT_FOUND) return "Ese docente ya no existe. Recarga la página.";
  if (cause.status === TOO_MANY_REQUESTS) return "Enviaste demasiadas invitaciones seguidas. Espera un minuto e intenta de nuevo.";
  return `${FAILED} Intenta de nuevo.`;
}

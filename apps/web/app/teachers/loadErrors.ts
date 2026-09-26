import { ConfigError } from "../config/api";

const FORBIDDEN = 403;
const CONFLICT = 409;

const NO_PERMISSION = "No tienes permiso para cambiar la carga académica.";
const CONCURRENT_CHANGE = "Alguien cambió esta carga al mismo tiempo; recargamos la lista, inténtalo de nuevo.";

/** The server answered that someone else changed the load first. */
export function isConcurrentChange(cause: unknown): boolean {
  return cause instanceof ConfigError && cause.status === CONFLICT;
}

/**
 * What a failed teaching-load write tells the person. A server answer is
 * never blamed on the connection: known statuses get plain Spanish, anything
 * else keeps the server's own words after the action that failed.
 */
export function describeLoadWriteError(cause: unknown, failedAction: string): string {
  if (!(cause instanceof ConfigError)) return `${failedAction} Revisa tu conexión e intenta de nuevo.`;
  if (cause.status === FORBIDDEN) return NO_PERMISSION;
  if (cause.status === CONFLICT) return CONCURRENT_CHANGE;
  return cause.detail !== "" ? `${failedAction} ${cause.detail}` : `${failedAction} Intenta de nuevo.`;
}

import { VoucherSendError, type VoucherSendReason } from "../../api";

/**
 * What the voucher form reads from a family, and what it tells them back. Pure:
 * the screen only wires these to its fields.
 */

const CENTS_PER_PESO = 100;
const SECONDS_PER_MINUTE = 60;

// Morelia has kept UTC−6 all year since Mexico dropped daylight saving (2022).
// A fixed offset, not Intl: Hermes ignored Intl options on the phone before
// (ISSUE-T14-106), and the school's day must not depend on that.
const SCHOOL_UTC_OFFSET_MS = -6 * 60 * 60 * 1000;

/** "2,500.50" / "$2500.5": thousands with commas, decimals with a point. */
const POINT_DECIMALS = /^(\d{1,3}(?:,\d{3})+|\d+)(?:\.(\d{1,2}))?$/;
/** "2500,50": a comma for decimals, then no thousands separator at all. */
const COMMA_DECIMALS = /^(\d+),(\d{1,2})$/;
/** 21/10/2026, 21-10-2026 or 21102026. */
const DAY_INPUT = /^(\d{1,2})[/-](\d{1,2})[/-](\d{4})$|^(\d{2})(\d{2})(\d{4})$/;

/**
 * Cents from what the family typed, with or without separators. Null when the
 * field is empty (the amount is optional); "invalid" when it is not an amount
 * above zero — "1.234" too, which could be either.
 */
export function parseAmountCents(typed: string): number | null | "invalid" {
  const clean = typed.replace(/[\s$]/g, "");
  if (clean === "") return null;
  const match = POINT_DECIMALS.exec(clean) ?? COMMA_DECIMALS.exec(clean);
  if (match === null) return "invalid";
  const pesos = Number.parseInt(match[1].replace(/,/g, ""), 10);
  const cents = Number.parseInt((match[2] ?? "").padEnd(2, "0"), 10);
  const total = pesos * CENTS_PER_PESO + cents;
  return total > 0 ? total : "invalid";
}

/** The school's today (America/Mexico_City), YYYY-MM-DD. */
export function schoolToday(now: Date = new Date()): string {
  return new Date(now.getTime() + SCHOOL_UTC_OFFSET_MS).toISOString().slice(0, 10);
}

/** A YYYY-MM-DD day as the family writes it: DD/MM/AAAA. */
export function dayInputOf(iso: string): string {
  const [year, month, day] = iso.split("-");
  return `${day}/${month}/${year}`;
}

/**
 * The payment day from the field. Null when empty (optional); "invalid" when it
 * is no real date; "future" when it is after the school's today.
 */
export function parseDayInput(typed: string, today: string): string | null | "invalid" | "future" {
  const clean = typed.trim();
  if (clean === "") return null;
  const match = DAY_INPUT.exec(clean);
  if (match === null) return "invalid";
  const [day, month, year] = (match[1] !== undefined ? match.slice(1, 4) : match.slice(4, 7)).map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCFullYear() !== year || date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) return "invalid";
  const iso = date.toISOString().slice(0, 10);
  return iso > today ? "future" : iso;
}

const SEND_MESSAGES: Record<Exclude<VoucherSendReason, "rate_limited">, string> = {
  too_large: "El archivo pesa más de 5 MB. Elige una foto más ligera o un PDF más pequeño.",
  limit: "Ya tienes 3 comprobantes por revisar para este alumno. Espera a que la escuela los revise.",
  conflict: "La escuela no pudo tomar este comprobante. Revisa la lista de abajo o pregunta en la escuela.",
  invalid: "Revisa el archivo, el importe y la fecha e inténtalo de nuevo.",
  session: "Tu sesión terminó. Vuelve a iniciar sesión para enviar el comprobante.",
  unavailable: "El envío de comprobantes no está disponible por ahora.",
  network: "No pudimos enviar el comprobante. Revisa tu conexión e inténtalo de nuevo.",
  server: "La escuela no pudo recibir el comprobante en este momento. Inténtalo más tarde.",
};

function waitPhrase(seconds: number | undefined): string {
  if (seconds === undefined) return "unos minutos";
  const minutes = Math.ceil(seconds / SECONDS_PER_MINUTE);
  return minutes === 1 ? "1 minuto" : `${minutes} minutos`;
}

/** The sentence a family reads when a voucher was not taken. */
export function sendErrorMessage(error: unknown): string {
  if (!(error instanceof VoucherSendError)) return SEND_MESSAGES.server;
  if (error.reason === "rate_limited") {
    return `Enviaste varios comprobantes seguidos. Espera ${waitPhrase(error.retryAfterSeconds)} e inténtalo de nuevo.`;
  }
  return SEND_MESSAGES[error.reason];
}

import type { Recurrence } from "../../finance/api";
import { pesosToCents } from "../../_lib/money";

/** How the school says how often a concept is charged. */
export const RECURRENCE_LABEL: Record<Recurrence, string> = { once: "Una vez", monthly: "Mensual" };

export const RECURRENCE_OPTIONS: readonly { value: Recurrence; label: string }[] = [
  { value: "once", label: RECURRENCE_LABEL.once },
  { value: "monthly", label: RECURRENCE_LABEL.monthly },
];

export const BAD_AMOUNT = "Escribe el monto en pesos, por ejemplo 80 o 1,850.50.";

/** A typed peso amount the server can take, or the message that says why not. */
export function amountProblem(text: string): string | undefined {
  if (text.trim() === "") return undefined;
  return Number.isNaN(pesosToCents(text)) ? BAD_AMOUNT : undefined;
}

/** The recurrence a select sent back, never a value the API does not know. */
export function toRecurrence(value: string): Recurrence {
  return value === "once" ? "once" : "monthly";
}

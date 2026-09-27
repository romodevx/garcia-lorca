import { GradeRequestError } from "../../../grades/monthly";
import { ALREADY_CLOSED, ALREADY_OPEN, LEVEL_NOT_GRADED, TERM_WITHOUT_MONTHS } from "../../../grades/periods";

/**
 * What a refused close or reopen means for the screen: either the period is
 * already where the director wanted it (someone else got there first — reload
 * and say so), or a message to read in the dialog.
 */
export type Refusal = { kind: "stale"; toast: string } | { kind: "message"; text: string };

const CLOSE_FAILED = "No se pudo cerrar el periodo. Intenta de nuevo.";
const REOPEN_FAILED = "No se pudo reabrir el periodo. Intenta de nuevo.";
export const WITHOUT_MONTHS_TEXT = "Este periodo no tiene meses asignados. Asígnalos en Configuración → Periodos y vuelve a cerrarlo.";
export const NOT_GRADED_TEXT = "Este nivel no se califica con números: sus periodos no emiten boleta.";

function serverText(err: unknown, fallback: string): string {
  return err instanceof GradeRequestError && err.detail !== "" ? err.detail : fallback;
}

const codeOf = (err: unknown): string | undefined => (err instanceof GradeRequestError ? err.code : undefined);

export function closeRefusal(err: unknown): Refusal {
  switch (codeOf(err)) {
    case ALREADY_CLOSED:
      return { kind: "stale", toast: "El periodo ya estaba cerrado" };
    case TERM_WITHOUT_MONTHS:
      return { kind: "message", text: WITHOUT_MONTHS_TEXT };
    case LEVEL_NOT_GRADED:
      return { kind: "message", text: NOT_GRADED_TEXT };
    default:
      return { kind: "message", text: serverText(err, CLOSE_FAILED) };
  }
}

export function reopenRefusal(err: unknown): Refusal {
  if (codeOf(err) === ALREADY_OPEN) return { kind: "stale", toast: "El periodo ya estaba abierto" };
  return { kind: "message", text: serverText(err, REOPEN_FAILED) };
}

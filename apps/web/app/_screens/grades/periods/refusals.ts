import { ALREADY_CLOSED, ALREADY_OPEN, LEVEL_NOT_GRADED, PERIOD_NOT_CONFIGURED } from "../../../grades/periods";
import { GradeRequestError } from "../../../grades/requestError";

/**
 * What a refused close or reopen means for the screen: either the period is
 * already where the director wanted it (someone else got there first — reload
 * and say so), or a message to read in the dialog.
 */
export type Refusal = { kind: "stale"; toast: string } | { kind: "message"; text: string };

const CLOSE_FAILED = "No se pudo cerrar el periodo. Intenta de nuevo.";
const REOPEN_FAILED = "No se pudo reabrir el periodo. Intenta de nuevo.";
/** Where a missing evaluation period is given, said after the server's sentence naming the group. */
export const CONFIGURE_PERIOD_POINTER = "Dale ese periodo en Configuración › Periodos por grado y vuelve a cerrarlo.";
const NOT_CONFIGURED_FALLBACK = "A un grupo le falta este periodo de evaluación.";
export const NOT_GRADED_TEXT = "Este nivel no se califica con números: sus periodos no emiten boleta.";

function serverText(err: unknown, fallback: string): string {
  return err instanceof GradeRequestError && err.detail !== "" ? err.detail : fallback;
}

const codeOf = (err: unknown): string | undefined => (err instanceof GradeRequestError ? err.code : undefined);

export function closeRefusal(err: unknown): Refusal {
  switch (codeOf(err)) {
    case ALREADY_CLOSED:
      return { kind: "stale", toast: "El periodo ya estaba cerrado" };
    case PERIOD_NOT_CONFIGURED:
      return { kind: "message", text: `${serverText(err, NOT_CONFIGURED_FALLBACK)}. ${CONFIGURE_PERIOD_POINTER}` };
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

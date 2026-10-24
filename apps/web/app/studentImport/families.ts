/**
 * The words of the import's "Familias" block (PLAN-familias-importacion I3).
 * Pure: the commit's `families` and the undo's counts in, Spanish out.
 */
import { StudentImportError, type StudentImportFamilies, type StudentImportFamiliesUndo } from "./api";

const families = (n: number): string => `${n} ${n === 1 ? "familia" : "familias"}`;
const existed = (n: number): string => (n === 1 ? "que ya existía" : "que ya existían");

export const UNCONFIRMED = "No pudimos confirmar todas las familias; lo que no se armó queda como sugerencia.";
export const NONE_LIKELY = "No encontramos familias muy probables.";
/** The headline once an undo took everything back: the step must not still say "Armamos". */
export const UNDONE_HEADLINE = "Deshicimos las familias que armamos.";

/** The block's first sentence. */
export function familiesHeadline(f: StudentImportFamilies): string {
  // A cut build may have written families it never got to report.
  if (!f.built) return UNCONFIRMED;
  const created = f.created.length;
  const joined = f.joined.length;
  if (created === 0 && joined === 0) return NONE_LIKELY;
  if (created === 0) return `Sumamos alumnos a ${families(joined)} ${existed(joined)}.`;
  const made = `Armamos ${families(created)} automáticamente`;
  return joined === 0 ? `${made}.` : `${made} y sumamos alumnos a ${families(joined)} ${existed(joined)}.`;
}

/** Undo is offered whenever something may have been built: undoing is idempotent and safe. */
export function canUndoFamilies(f: StudentImportFamilies): boolean {
  return !f.built || f.created.length + f.joined.length > 0;
}

/** A family the block can name and link to. */
export interface NamedFamily {
  id: string;
  name: string;
}

/** The families whose names the gateway sent (students:read), created first. Names only, never contacts. */
export function builtFamilyNames(f: StudentImportFamilies): NamedFamily[] {
  return [...f.created, ...f.joined].flatMap((x) => (x.name === undefined || x.name === "" ? [] : [{ id: x.id, name: x.name }]));
}

export function reviewLinkText(n: number): string {
  return n === 1 ? "1 posible familia por revisar" : `${n} posibles familias por revisar`;
}

const UNDO_GENERIC = "No pudimos deshacer las familias. Intenta de nuevo.";

const UNDO_REFUSALS: Readonly<Record<number, string>> = {
  403: "Tu cuenta no tiene permiso para deshacer familias.",
  // A flag turned off since the commit: the route is gone until the page reloads.
  404: "Esta función ya no está disponible; recarga la página.",
  // The gateway's per-user limiter on undo.
  429: "Demasiados intentos seguidos. Espera un momento y vuelve a intentar.",
};

/** The undo's refusal in words; anything without a known status keeps the generic line. */
export function undoFailureText(err: unknown): string {
  if (!(err instanceof StudentImportError)) return UNDO_GENERIC;
  return UNDO_REFUSALS[err.status] ?? UNDO_GENERIC;
}

const pupilsOut = (n: number): string => (n === 1 ? "Sacamos a 1 alumno de su familia" : `Sacamos a ${n} alumnos de sus familias`);
const emptied = (n: number): string => (n === 1 ? "deshicimos 1 familia que quedó vacía" : `deshicimos ${n} familias que quedaron vacías`);

/**
 * What the undo did, sentence by sentence. `removed` counts every pupil taken
 * out (of new and of pre-existing families alike) and `dissolved` the families
 * left empty, so each sentence says only what those two numbers prove.
 */
export function undoOutcome(o: StudentImportFamiliesUndo): string[] {
  const lines: string[] = [];
  if (o.removed > 0 && o.dissolved > 0) lines.push(`${pupilsOut(o.removed)} y ${emptied(o.dissolved)}.`);
  else if (o.removed > 0) lines.push(`${pupilsOut(o.removed)}.`);
  else if (o.dissolved > 0) lines.push(`${emptied(o.dissolved).replace(/^d/, "D")}.`);
  else if (o.keptWithAccess === 0) lines.push("Ya no quedaba nada por deshacer.");
  if (o.keptWithAccess > 0) lines.push(keptLine(o.keptWithAccess));
  return lines;
}

/** The batch lives only in this page's memory: the way back must be a place that exists later. */
function keptLine(n: number): string {
  const one = n === 1;
  return `${families(n)} ya ${one ? "tiene" : "tienen"} acceso a la app y no se ${one ? "tocó" : "tocaron"}: para ${one ? "deshacerla" : "deshacerlas"}, quita el acceso y saca a los alumnos desde Alumnos › Familias.`;
}

/** Nothing left to take back: the undo can retire and the headline change. */
export const undoIsComplete = (o: StudentImportFamiliesUndo | undefined): boolean => o !== undefined && o.keptWithAccess === 0;

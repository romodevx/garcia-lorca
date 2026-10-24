/**
 * The words of the import's "Familias" block (PLAN-familias-importacion I3).
 * Pure: the commit's `families` and the undo's counts in, Spanish out.
 */
import type { StudentImportFamilies, StudentImportFamiliesUndo } from "./api";

const families = (n: number): string => `${n} ${n === 1 ? "familia" : "familias"}`;
const existed = (n: number): string => (n === 1 ? "que ya existía" : "que ya existían");

export const UNCONFIRMED = "No pudimos confirmar todas las familias; lo que no se armó queda como sugerencia.";
export const NONE_LIKELY = "No encontramos familias muy probables.";

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

/** The names the gateway sent (students:read), created first. Names only, never contacts. */
export function builtFamilyNames(f: StudentImportFamilies): string[] {
  return [...f.created, ...f.joined].flatMap((x) => (x.name === undefined || x.name === "" ? [] : [x.name]));
}

export function reviewLinkText(n: number): string {
  return n === 1 ? "1 posible familia por revisar" : `${n} posibles familias por revisar`;
}

/** What the undo did, sentence by sentence. */
export function undoOutcome(o: StudentImportFamiliesUndo): string[] {
  const lines: string[] = [];
  if (o.dissolved > 0) lines.push(`Deshicimos ${families(o.dissolved)}.`);
  else if (o.removed > 0) lines.push(`Sacamos a ${o.removed} ${o.removed === 1 ? "alumno" : "alumnos"} de las familias a las que se sumaron.`);
  else if (o.keptWithAccess === 0) lines.push("Ya no quedaba nada por deshacer.");
  if (o.keptWithAccess > 0) {
    const one = o.keptWithAccess === 1;
    lines.push(`${families(o.keptWithAccess)} ya ${one ? "tiene" : "tienen"} acceso a la app y no se ${one ? "tocó" : "tocaron"}; quita el acceso desde la familia primero.`);
  }
  return lines;
}

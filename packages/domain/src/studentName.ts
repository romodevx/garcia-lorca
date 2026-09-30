/**
 * The one place a pupil's name is put together and ordered (T3).
 *
 * The school reads its lists surname first — "Pérez Núñez Ana Sofía" — and
 * alphabetically by surname, in Spanish: accents and case do not move a pupil,
 * ñ sorts after n. Every screen of the web panel and the families' app formats
 * and sorts through here; the Go twin is `pkg/studentname`.
 *
 * `lastName` is the PATERNAL surname. A pupil captured before the maternal field
 * existed may carry both surnames in it; it is shown as written.
 */
export interface StudentNameParts {
  firstName?: string | null;
  lastName?: string | null;
  maternalLastName?: string | null;
}

const clean = (s: string | null | undefined): string => (s ?? "").trim();

/** "Paterno Materno Nombre(s)", skipping the empty pieces. */
export function formatStudentName(s: StudentNameParts): string {
  return [clean(s.lastName), clean(s.maternalLastName), clean(s.firstName)].filter((p) => p !== "").join(" ");
}

/** Where space is tight (the families' app header): "Paterno PrimerNombre". */
export function studentShortName(s: StudentNameParts): string {
  const firstGiven = clean(s.firstName).split(/\s+/)[0] ?? "";
  return [clean(s.lastName), firstGiven].filter((p) => p !== "").join(" ");
}

/** The avatar monogram: first name's initial, then the paternal surname's. */
export function studentInitials(s: StudentNameParts): string {
  return `${clean(s.firstName).charAt(0)}${clean(s.lastName).charAt(0)}`.toUpperCase();
}

// Strength 1 ("base"): accents and case are ignored, ñ stays its own letter.
const collator = new Intl.Collator("es", { sensitivity: "base" });

/** Unnamed goes last; otherwise the Spanish collation decides. */
function compareText(a: string, b: string): number {
  if ((a === "") !== (b === "")) return a === "" ? 1 : -1;
  return collator.compare(a, b);
}

/** Orders by paternal surname, then maternal, then first name. */
export function compareStudents(a: StudentNameParts, b: StudentNameParts): number {
  const unnamedA = formatStudentName(a) === "";
  if (unnamedA !== (formatStudentName(b) === "")) return unnamedA ? 1 : -1;
  return (
    collator.compare(clean(a.lastName), clean(b.lastName)) ||
    collator.compare(clean(a.maternalLastName), clean(b.maternalLastName)) ||
    collator.compare(clean(a.firstName), clean(b.firstName))
  );
}

/** A sorted copy (stable: identical names keep their order). */
export function sortStudents<T extends StudentNameParts>(list: readonly T[]): T[] {
  return [...list].sort(compareStudents);
}

/**
 * Orders names that arrive already formatted (a roster row, a stored snapshot)
 * with the same collation.
 */
export function compareStudentNames(a: string, b: string): number {
  return compareText(a.trim(), b.trim());
}

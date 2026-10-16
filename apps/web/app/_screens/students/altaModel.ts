import { formatStudentName } from "../../_design/students/studentName";
import { API_URL } from "../../api-url";
import type { LabelledGroup } from "../../config/api";
import { addNote, createStudent, enroll, enrollIntoGrade, StudentsError, type CreateStudentInput } from "../../students/api";

/**
 * What the alta of one pupil validates, sends and says (T9). Pure, apart from
 * `saveAlta`, which is the two-call orchestration the page awaits.
 */

/**
 * Marks a failure that happened AFTER the pupil was created, while placing them.
 *
 * The distinction is the whole reason it exists: the page has to tell somebody
 * that their pupil is safe and only the placement failed, or they re-type the
 * form and meet "ya existe ese CURP".
 */
export class PlacementFailed extends Error {
  override readonly cause: unknown;
  /** The first note failed too: a retry cannot write it (the CURP is taken). */
  readonly noteFailed: boolean;
  /** The pupil that WAS created: a retry places them instead of creating twice. */
  readonly studentId: string;

  constructor(cause: unknown, noteFailed = false, studentId = "") {
    super("placement failed");
    this.name = "PlacementFailed";
    this.cause = cause;
    this.noteFailed = noteFailed;
    this.studentId = studentId;
  }
}

/** Everything one press of "Crear alumno" asks for (T9 added group and entry day). */
export interface AltaRequest {
  student: CreateStudentInput;
  gradeLevelId: string;
  /** "" lets capacity choose the group. */
  groupId: string;
  /** How the chosen group reads in the confirmation, "1° PRIMARIA · B". */
  groupLabel: string;
  /** "" means today; anything else is a past day the server bills from. */
  enrolledOn: string;
  note: string;
  /** Set on a retry: the pupil exists already, only the placement is left. */
  createdStudentId?: string;
}

export interface AltaResult {
  studentId: string;
  /** Where the pupil landed, as the confirmation names it. */
  groupName: string;
  noteFailed: boolean;
}

export const NOTE_LOST = "La nota no se guardó: agrégala en su ficha.";

/** The shape the alta validates before it asks the server. */
export interface QuickAddDraft {
  curp: string;
  firstName: string;
  /** The paternal surname. The maternal one is optional and never "missing". */
  lastName: string;
  guardianName: string;
  /** The config grade the pupil joins. */
  gradeLevelId?: string;
}

/** A CURP is exactly eighteen characters. The server checks its shape too. */
const CURP_LENGTH = 18;
const MIN_NAME = 2;
const MIN_GUARDIAN = 3;

/**
 * What the form is still waiting for, or undefined when it is ready: one field
 * at a time, in reading order, so the answer matches where the eye already is.
 */
export function missingFieldMessage(draft: QuickAddDraft): string | undefined {
  return Object.values(missingFields(draft))[0];
}

/**
 * The same, per field, so ALL of them are marked at once. Each message keeps
 * its own words: a 17-character CURP is not "empty", and saying how many it
 * has is what this screen always did well. Keys come in reading order.
 */
export function missingFields(draft: QuickAddDraft): Record<string, string> {
  const found: Record<string, string> = {};
  const curp = draft.curp.trim();
  if (curp === "") {
    found.curp = "Escribe el CURP del alumno.";
  } else if (curp.length !== CURP_LENGTH) {
    found.curp = `El CURP debe tener ${CURP_LENGTH} caracteres; llevas ${curp.length}.`;
  }
  // Surname before first name: the order the form asks for them (T3).
  if (draft.lastName.trim().length < MIN_NAME) found.lastName = "Escribe el apellido paterno del alumno.";
  if (draft.firstName.trim().length < MIN_NAME) found.firstName = "Escribe el nombre del alumno.";
  if (draft.guardianName.trim().length < MIN_GUARDIAN) found.guardianName = "Escribe el nombre del tutor.";
  if (draft.gradeLevelId !== undefined && draft.gradeLevelId === "") {
    found.gradeLevelId = "Elige el nivel y el grado al que entra.";
  }
  return found;
}

/**
 * What the server said, in words a person can act on. A duplicate and a
 * malformed CURP need different advice, and a server that fell over needs
 * advice about neither.
 */
export function createFailureMessage(err: unknown): string {
  if (err instanceof StudentsError) {
    if (err.isConflict) return "Ya existe un alumno con ese CURP.";
    if (err.isInvalid) return "Revisa el CURP: son 18 caracteres con el formato oficial.";
  }
  return "No se pudo crear el alumno. Intenta de nuevo en un momento.";
}

const HTTP_FORBIDDEN = 403;

/**
 * What went wrong PLACING a pupil who, by then, already exists. Saying so is
 * the point: without it somebody re-types the form and meets "ya existe".
 */
export function placementFailureMessage(err: unknown): string {
  const tail = " El alumno sí se creó: inscríbelo desde su ficha cuando haya lugar.";
  // A refused entry day is fixed right here, and the retry only places (T9).
  const retryTail = " El alumno sí se creó: corrige y presiona «Crear alumno» otra vez.";
  if (err instanceof StudentsError) {
    if (err.isConflict) return "Ese grado ya no tiene lugares. Abre otro grupo en Configuración." + tail;
    if (err.isInvalid) return (err.detail !== "" ? err.detail : "No se pudo inscribir en ese grado.") + retryTail;
    if (err.status === HTTP_FORBIDDEN && err.detail !== "") return err.detail + retryTail;
  }
  return "No se pudo inscribir al alumno en ese grado." + tail;
}

/** The whole failure line: the placement's words, plus the lost note if any. */
export function altaFailureMessage(err: unknown): string {
  if (!(err instanceof PlacementFailed)) return createFailureMessage(err);
  return [placementFailureMessage(err.cause), err.noteFailed ? NOTE_LOST : ""].filter((m) => m !== "").join(" ");
}

/**
 * How a group reads in the picker. Its capacity is the group's SIZE, not its
 * free seats — "25 lugares" read as 25 left — and 0 means nobody sized it yet.
 */
export function groupOptionLabel(group: LabelledGroup): string {
  return group.capacity > 0 ? `${group.name} · cupo ${group.capacity}` : `${group.name} · sin cupo definido`;
}

/** The confirmation, on the list's toast or the page's own status line. */
export function createdMessage(input: CreateStudentInput, result: AltaResult): string {
  const base = `Alumno creado · ${formatStudentName(input)} quedó en ${result.groupName}`;
  return result.noteFailed ? `${base}, pero la nota no se guardó: agrégala en su ficha` : base;
}

// A chosen group goes to POST /enrollments, which answers with the enrolment
// only, so the request's own label names it; otherwise capacity chooses (T9).
async function place(studentId: string, req: AltaRequest): Promise<string> {
  if (req.groupId === "") {
    return (await enrollIntoGrade(API_URL, studentId, req.gradeLevelId, req.enrolledOn)).groupName;
  }
  await enroll(API_URL, { studentId, groupId: req.groupId, enrolledOn: req.enrolledOn });
  return req.groupLabel;
}

/**
 * Creates the pupil (unless a retry says they exist), writes the first note,
 * then places them. The placement is the call that fails on a full grade — by
 * then the pupil exists — so its failure travels as PlacementFailed.
 */
export async function saveAlta(req: AltaRequest): Promise<AltaResult> {
  let studentId = req.createdStudentId;
  let noteFailed = false;
  if (studentId === undefined) {
    const created = await createStudent(API_URL, req.student);
    studentId = created.id;
    // The note goes BEFORE placement: a retry must not write it twice, and a
    // failed note does not undo the pupil.
    noteFailed = req.note !== "" && !(await addNote(API_URL, created.id, req.note).then(() => true, () => false));
  }
  try {
    return { studentId, groupName: await place(studentId, req), noteFailed };
  } catch (err) {
    throw new PlacementFailed(err, noteFailed, studentId);
  }
}

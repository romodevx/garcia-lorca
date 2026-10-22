/**
 * Every sentence the pupil migration says (PLAN-student-import.md, "Flujo del
 * usuario"). The gateway, students and config answer with stable codes; the
 * screen words them here, once, so a new code shows a Spanish fallback and
 * never a raw `snake_case` token.
 */
import { StudentImportError, type ImportRow, type ImportRowIssue, type IssueField, type OverrideName, type RowFieldError } from "./api";

/** Codes → Spanish. Skip reasons first, then the warnings an imported row can carry. */
export const REASON_TEXT: Readonly<Record<string, string>> = {
  // left out by the spreadsheet itself (gateway)
  status_withdrawn: "Baja en el sistema anterior",
  status_unknown: "Estado del alumno no reconocido",
  grade_invalid: "Grado no reconocido",
  group_invalid: "Grupo no reconocido",
  level_missing: "Sin escolaridad en el archivo",
  level_invalid: "Nombre de nivel demasiado largo",
  name_unreadable: "No se pudo leer el nombre",
  level_unresolved: "Falta elegir el nivel",
  not_answered: "El sistema no respondió por esta fila",
  // the destination (config)
  level_not_found: "El nivel elegido ya no existe",
  level_being_deleted: "El nivel se está eliminando",
  invalid_placement: "Grado o grupo inválido para ese nivel",
  // judged by students
  already_exists: "Ya registrado con esta CURP",
  already_imported: "Ya importado en este grupo",
  curp_invalid: "CURP vacía o inválida",
  curp_duplicated: "CURP repetida en el archivo",
  name_required: "Falta el nombre",
  group_not_found: "El grupo destino no existe",
  group_full: "El grupo destino está lleno",
  // warnings: the row goes in, something was dropped or guessed
  birth_date_from_curp: "Fecha de nacimiento tomada de la CURP",
  birth_date_invalid: "Fecha de nacimiento ilegible",
  surnames_curp_mismatch: "Apellidos no coinciden con la CURP",
  surnames_ambiguous: "Revisa cómo se separaron los apellidos",
  name_without_comma: "Nombre sin coma: el orden se dedujo con la CURP",
  first_name_curp_mismatch: "El nombre no coincide con la CURP",
  bank_reference_taken: "Referencia BBVA ya usada por otro alumno",
  bank_reference_invalid: "Sin referencia BBVA (S/R)",
  phone_invalid: "Teléfono no tiene 10 dígitos",
  email_invalid: "Correo inválido",
  state_unknown: "Estado del domicilio no reconocido",
  postal_code_invalid: "Código postal inválido",
  address_truncated: "Domicilio recortado por largo",
  // the office's own decision (module_import_review)
  omitted: "Omitida en la revisión",
};

/** For a code this screen does not know yet. */
export const GENERIC_REASON = "Revisa esta fila en el archivo";

export function reasonText(code: string): string {
  return REASON_TEXT[code] ?? GENERIC_REASON;
}

/** The row's reasons in reading order: why it is left out, then what was dropped. */
export function rowReasons(row: Pick<ImportRow, "reason" | "warnings">): string[] {
  const codes = [...(row.reason !== undefined && row.reason !== "" ? [row.reason] : []), ...(row.warnings ?? [])];
  return codes.map(reasonText);
}

export const STALE_PREVIEW = "Algo cambió desde la vista previa; revísala de nuevo.";
export const NOT_XLSX = "Este archivo no es un Excel (.xlsx). Si viene como .xls, ábrelo en Excel y guárdalo como .xlsx.";
export const TOO_LARGE = "El archivo pasa de 1,000 filas o 2 MB. Sepáralo por nivel.";
export const ONE_FILE = "Sube un archivo a la vez";
export const PREVIEW_BANNER = "Vista previa: todavía no se ha guardado nada.";
export const CUT_UNKNOWN =
  "La conexión se cortó mientras se importaba. Sube el mismo archivo otra vez: los que ya entraron aparecerán como «Ya estaba» y sólo se agregará el resto.";

export function missingColumn(column: string): string {
  return `Al archivo le falta la columna «${column}». Usa el reporte tal como lo exporta el sistema anterior.`;
}

/** What went wrong reading or importing the file, in words that point at a fix. */
export function failureText(err: unknown): string {
  if (!(err instanceof StudentImportError)) return "No pudimos comunicarnos con el sistema. Revisa tu conexión e intenta de nuevo.";
  switch (err.code) {
    case "FILE_NOT_XLSX":
      return NOT_XLSX;
    case "FILE_MISSING_COLUMN":
      return missingColumn(err.detail);
    case "FILE_EMPTY":
      return "La hoja no tiene alumnos. Usa el reporte tal como lo exporta el sistema anterior.";
    case "FILE_TOO_LARGE":
      return TOO_LARGE;
    case "PREVIEW_STALE":
      return STALE_PREVIEW;
    case "IMPORT_IN_PROGRESS":
      return "Ya hay una importación en curso para la escuela. Espera a que termine y vuelve a intentarlo.";
    case "LEVELS_UNRESOLVED":
      return "Falta elegir a dónde van algunos niveles.";
    case "IMPORT_ROWS_INVALID":
      return ROWS_INVALID;
    default:
      break;
  }
  if (err.status === 401) return "Tu sesión expiró. Vuelve a entrar.";
  if (err.status === 403) return "Tu cuenta no tiene permiso para migrar alumnos.";
  if (err.status === 404) return "La migración de alumnos está apagada.";
  return "No pudimos leer el archivo. Revisa tu conexión e intenta de nuevo.";
}

/** The O5 box above the primary button. */
export function confirmationText(cycleName: string): string {
  return `Se inscribirán en el ciclo ${cycleName} desde su primer día, sin cargo de inscripción. Las colegiaturas mensuales sí se les cobrarán como a todos, desde el primer mes del ciclo; lo que ya venció antes de hoy, sin recargo. Una importación no se deshace de golpe: cada alumno se da de baja uno por uno.`;
}

function enrollments(n: number): string {
  return `${n} ${n === 1 ? "inscripción" : "inscripciones"}`;
}

/** T14, in the preview: earlier imports dated after the cycle's first day. */
export function toMoveText(count: number): string {
  const one = count === 1;
  return `${enrollments(count)} de una importación anterior ${one ? "se moverá" : "se moverán"} al inicio del ciclo, contando a los alumnos dados de baja. Los activos se cobrarán desde el primer mes, como a todos; lo que venció antes de su importación, sin recargo.`;
}

/** The primary button when the file only repairs earlier imports. */
export function moveButtonLabel(count: number): string {
  return `Mover ${enrollments(count)} al inicio del ciclo`;
}

/** T14, once done. */
export function movedText(count: number): string {
  return `${enrollments(count)} ${count === 1 ? "movida" : "movidas"} al inicio del ciclo.`;
}

/** "Primaria", "Preescolar y Primaria", "A, B y C". */
export function joinNames(names: readonly string[]): string {
  if (names.length <= 1) return names[0] ?? "";
  return `${names.slice(0, -1).join(", ")} y ${names[names.length - 1]}`;
}

export function pupils(n: number): string {
  return `${n} ${n === 1 ? "alumno" : "alumnos"}`;
}

/** "Importar 297 alumnos a Primaria" — never just "Confirmar". */
export function importButtonLabel(count: number, levelNames: readonly string[]): string {
  return `Importar ${pupils(count)} a ${joinNames(levelNames)}`;
}

export function busyText(count: number, moves = 0): string {
  if (count === 0 && moves > 0) return `Moviendo ${enrollments(moves)} al inicio del ciclo… no cierres esta ventana`;
  return `Importando ${pupils(count)}… no cierres esta ventana`;
}

export function allAlreadyText(count: number): string {
  return `Estos ${count} alumnos ya estaban registrados (misma CURP). No se duplicó nada.`;
}

export function cutWithCount(imported: number, expected: number): string {
  return `Se importaron ${imported} de ${expected} y la conexión se cortó. Sube el mismo archivo otra vez: los que ya entraron aparecerán como «Ya estaba» y sólo se agregará el resto.`;
}

export function migratedText(levelNames: readonly string[]): string {
  return `${joinNames(levelNames)} quedó migrada.`;
}

// --- the import review (module_import_review, PLAN-import-avisos) ------------

/** What the import changes or drops in a value, per issue code. */
export const ISSUE_TEXT: Readonly<Record<string, string>> = {
  birth_date_from_curp: "La fecha no coincide con la CURP; se usará la de la CURP",
  birth_date_invalid: "Fecha de nacimiento ilegible",
  phone_invalid: "Teléfono incompleto",
  curp_missing: "Sin CURP",
  curp_invalid: "CURP inválida",
  email_invalid: "Correo inválido",
  postal_code_invalid: "Código postal inválido",
  state_unknown: "Estado no reconocido",
  bank_reference_invalid: "Referencia BBVA inválida",
  bank_reference_taken: "Referencia BBVA de otro alumno",
  surnames_curp_mismatch: "Los apellidos no coinciden con la CURP",
  surnames_ambiguous: "No está claro cuál es cada apellido",
  name_without_comma: "Nombre sin coma: el orden se dedujo con la CURP",
  first_name_curp_mismatch: "El nombre no coincide con la CURP",
  address_truncated: "Domicilio demasiado largo; se recortó",
};

export const GENERIC_ISSUE = "Revisa este dato";

export function issueText(code: string): string {
  return ISSUE_TEXT[code] ?? GENERIC_ISSUE;
}

/** The value an issue is about, as the office calls it. */
export const ISSUE_FIELD_LABEL: Readonly<Record<IssueField, string>> = {
  birth_date: "Fecha de nacimiento",
  phone_home: "Teléfono de casa",
  phone_mobile: "Celular",
  email: "Correo",
  postal_code: "Código postal",
  state: "Estado",
  bank_reference: "Referencia BBVA",
  surnames: "Apellidos",
  first_name: "Nombre",
  address: "Domicilio",
  curp: "CURP",
};

/** "Celular: Teléfono incompleto (en el Excel: «55 12»)". */
export function issueLine(issue: ImportRowIssue): string {
  const said = issue.original !== "" ? ` (en el Excel: «${issue.original}»)` : "";
  return `${ISSUE_FIELD_LABEL[issue.field] ?? issue.field}: ${issueText(issue.code)}${said}`;
}

/** On the pupil's file: what the Excel said and what was kept; "" when the file said nothing. */
export function issueOutcome(original: string, stored: string): string {
  if (original === "") return "";
  return stored !== "" ? `En el Excel decía «${original}»; se guardó «${stored}».` : `En el Excel decía «${original}»; no se guardó.`;
}

/** The label of each correction field in the preview's inline editor. */
export const OVERRIDE_LABEL: Readonly<Record<OverrideName, string>> = {
  curp: "CURP",
  birthDate: "Fecha de nacimiento",
  homePhone: "Teléfono de casa",
  mobilePhone: "Celular",
  email: "Correo",
  postalCode: "Código postal",
  state: "Estado",
  bankReference: "Referencia BBVA",
  municipality: "Municipio",
  street: "Calle y número",
  fullName: "Nombre completo",
};

/** Why the server refused a correction (422 IMPORT_ROWS_INVALID). */
const ROW_ERROR_TEXT: Readonly<Record<string, string>> = {
  curp_invalid: "la CURP no es válida",
  birth_date_invalid: "la fecha no es válida o es futura",
  phone_invalid: "el teléfono lleva 10 dígitos",
  postal_code_invalid: "el código postal lleva 5 dígitos",
  email_invalid: "el correo no es válido",
  state_unknown: "elige un estado de la lista",
  bank_reference_invalid: "sólo dígitos, hasta 20",
  bank_reference_taken: "esa referencia ya es de otro alumno",
  address_too_long: "máximo 120 caracteres",
  name_unreadable: "escríbelo como APELLIDOS, NOMBRES",
  curp_duplicated: "esa CURP ya la tiene otra fila u otro alumno",
};

/** Refusals about the row itself rather than one of its values. */
const ROW_LEVEL_ERROR_TEXT: Readonly<Record<string, string>> = {
  draft_has_curp: "Con una CURP válida no puede ir como borrador",
  action_unknown: "Vuelve a elegir qué hacer con esta fila",
  row_unknown: "Esta fila ya no está en el archivo; vuelve a subirlo",
  row_repeated: "Esta fila se decidió dos veces; vuelve a subir el archivo",
};

export function rowErrorText(err: RowFieldError): string {
  const rowLevel = ROW_LEVEL_ERROR_TEXT[err.code];
  if (rowLevel !== undefined) return rowLevel;
  const label = OVERRIDE_LABEL[err.field as OverrideName] ?? err.field;
  return `${label}: ${ROW_ERROR_TEXT[err.code] ?? "revisa este dato"}`;
}

export const ROWS_INVALID = "Algunas correcciones no se aceptaron. Revísalas en la tabla.";

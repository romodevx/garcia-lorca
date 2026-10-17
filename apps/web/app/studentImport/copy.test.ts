import { describe, expect, it } from "vitest";

import { StudentImportError } from "./api";
import {
  CUT_UNKNOWN,
  GENERIC_REASON,
  busyText,
  REASON_TEXT,
  confirmationText,
  cutWithCount,
  failureText,
  importButtonLabel,
  moveButtonLabel,
  movedText,
  toMoveText,
  reasonText,
  rowReasons,
} from "./copy";

describe("reasonText", () => {
  it.each([
    ["status_withdrawn", "Baja en el sistema anterior"],
    ["already_exists", "Ya registrado con esta CURP"],
    ["bank_reference_invalid", "Sin referencia BBVA (S/R)"],
    ["bank_reference_taken", "Referencia BBVA ya usada por otro alumno"],
    ["phone_invalid", "Teléfono no tiene 10 dígitos"],
    ["email_invalid", "Correo inválido"],
    ["birth_date_from_curp", "Fecha de nacimiento tomada de la CURP"],
    ["surnames_curp_mismatch", "Apellidos no coinciden con la CURP"],
    ["surnames_ambiguous", "Revisa cómo se separaron los apellidos"],
    ["level_unresolved", "Falta elegir el nivel"],
  ])("words %s as the plan says", (code, text) => {
    expect(reasonText(code)).toBe(text);
  });

  it("words every code the gateway, students and config can send", () => {
    const codes = [
      "status_withdrawn", "status_unknown", "grade_invalid", "group_invalid", "level_missing", "level_invalid", "name_unreadable",
      "level_unresolved", "not_answered", "level_not_found", "level_being_deleted", "invalid_placement",
      "already_exists", "already_imported", "curp_invalid", "curp_duplicated", "name_required", "group_not_found", "group_full",
      "birth_date_from_curp", "birth_date_invalid", "surnames_curp_mismatch", "surnames_ambiguous", "name_without_comma",
      "first_name_curp_mismatch", "bank_reference_taken", "bank_reference_invalid", "phone_invalid", "email_invalid",
      "state_unknown", "postal_code_invalid", "address_truncated",
    ];
    for (const code of codes) expect(REASON_TEXT[code], code).toBeDefined();
  });

  it("falls back to a Spanish sentence for a code it does not know, never the raw code", () => {
    expect(reasonText("something_new")).toBe(GENERIC_REASON);
  });
});

describe("rowReasons", () => {
  it("puts the reason first, then the warnings, all in Spanish", () => {
    expect(rowReasons({ reason: "already_exists", warnings: ["phone_invalid"] })).toEqual([
      "Ya registrado con esta CURP",
      "Teléfono no tiene 10 dígitos",
    ]);
    expect(rowReasons({})).toEqual([]);
  });
});

describe("failureText", () => {
  const err = (status: number, code: string | undefined, detail = ""): StudentImportError => new StudentImportError(status, detail, code);

  it.each([
    [err(400, "FILE_NOT_XLSX"), "Este archivo no es un Excel (.xlsx). Si viene como .xls, ábrelo en Excel y guárdalo como .xlsx."],
    [err(400, "FILE_MISSING_COLUMN", "CURP"), "Al archivo le falta la columna «CURP». Usa el reporte tal como lo exporta el sistema anterior."],
    [err(413, "FILE_TOO_LARGE"), "El archivo pasa de 1,000 filas o 2 MB. Sepáralo por nivel."],
    [err(409, "PREVIEW_STALE"), "Algo cambió desde la vista previa; revísala de nuevo."],
    [err(409, "IMPORT_IN_PROGRESS"), "Ya hay una importación en curso para la escuela. Espera a que termine y vuelve a intentarlo."],
  ])("words a coded refusal as the plan says", (error, text) => {
    expect(failureText(error)).toBe(text);
  });

  it("explains an empty sheet", () => {
    expect(failureText(err(400, "FILE_EMPTY"))).toMatch(/no tiene alumnos/);
  });

  it("explains permission, a switched-off module and anything else", () => {
    expect(failureText(err(403, undefined))).toMatch(/permiso/);
    expect(failureText(err(404, undefined))).toMatch(/apagad/);
    expect(failureText(new TypeError("Failed to fetch"))).toMatch(/conexión/);
  });
});

describe("the confirmation and the button", () => {
  it("states the O5 terms with the cycle's name", () => {
    expect(confirmationText("2026–2027")).toBe(
      "Se inscribirán en el ciclo 2026–2027 desde su primer día, sin cargo de inscripción. Las colegiaturas mensuales sí se les cobrarán como a todos, desde el primer mes del ciclo; lo que ya venció antes de hoy, sin recargo. Una importación no se deshace de golpe: cada alumno se da de baja uno por uno.",
    );
  });

  it("names how many and where — never just «Confirmar»", () => {
    expect(importButtonLabel(297, ["Primaria"])).toBe("Importar 297 alumnos a Primaria");
    expect(importButtonLabel(1, ["Maternal"])).toBe("Importar 1 alumno a Maternal");
    expect(importButtonLabel(12, ["Preescolar", "Primaria"])).toBe("Importar 12 alumnos a Preescolar y Primaria");
  });
});

describe("a cut commit", () => {
  it("says how many went in when the answer arrived", () => {
    expect(cutWithCount(180, 297)).toBe(
      "Se importaron 180 de 297 y la conexión se cortó. Sube el mismo archivo otra vez: los que ya entraron aparecerán como «Ya estaba» y sólo se agregará el resto.",
    );
  });

  it("still says re-uploading is safe when nothing arrived", () => {
    expect(CUT_UNKNOWN).toMatch(/Sube el mismo archivo otra vez/);
  });
});

describe("earlier imports moved to the cycle start (T14)", () => {
  it("says in the preview how many enrollments would move, and why it matters", () => {
    expect(toMoveText(12)).toBe(
      "12 inscripciones de una importación anterior se moverán al inicio del ciclo, contando a los alumnos dados de baja. Los activos se cobrarán desde el primer mes, como a todos; lo que venció antes de su importación, sin recargo.",
    );
    expect(toMoveText(1)).toBe(
      "1 inscripción de una importación anterior se moverá al inicio del ciclo, contando a los alumnos dados de baja. Los activos se cobrarán desde el primer mes, como a todos; lo que venció antes de su importación, sin recargo.",
    );
  });

  it("names the move on the button when nobody new comes in", () => {
    expect(moveButtonLabel(12)).toBe("Mover 12 inscripciones al inicio del ciclo");
    expect(moveButtonLabel(1)).toBe("Mover 1 inscripción al inicio del ciclo");
  });

  it("says what is happening while only moves are written", () => {
    expect(busyText(0, 2)).toBe("Moviendo 2 inscripciones al inicio del ciclo… no cierres esta ventana");
    expect(busyText(3, 2)).toBe("Importando 3 alumnos… no cierres esta ventana");
  });

  it("says how many moved once it is done", () => {
    expect(movedText(12)).toBe("12 inscripciones movidas al inicio del ciclo.");
    expect(movedText(1)).toBe("1 inscripción movida al inicio del ciclo.");
  });
});

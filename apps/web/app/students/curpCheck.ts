/**
 * CC-314: what a CURP says about itself, read as WARNINGS for the person
 * capturing it — never a block. The legacy import brings CURPs that fail these
 * checks and the school still has to register those pupils, so the server
 * keeps accepting any CURP of the official shape (students/domain/curp.go)
 * and only the screen warns.
 *
 * Three checks, all from RENAPO's structure:
 *   - the check digit (18th character) of the first seventeen;
 *   - the birth date it carries (YYMMDD, century from the 17th character: a
 *     digit is 19xx, a letter 20xx), against the birth date captured;
 *   - the sex it carries (11th character: H hombre, M mujer), against the
 *     file's when it has one (F/M; X and "not captured" are not compared). The
 *     alta captures no sex, so there only the first two apply.
 * The check digit is cross-checked with python-stdnum's mx.curp.
 */

import type { Sex } from "./api";

/** The file's sex each CURP letter stands for. */
const SEX_OF_CURP: Readonly<Record<string, { sex: Sex; word: string }>> = {
  H: { sex: "M", word: "hombre" },
  M: { sex: "F", word: "mujer" },
};

/** RENAPO's alphabet: a character's value is its index (Ñ sits after N). */
const CURP_ALPHABET = "0123456789ABCDEFGHIJKLMNÑOPQRSTUVWXYZ";

/** The official shape, the same pattern the students service enforces. */
const CURP_SHAPE = /^[A-Z][AEIOUX][A-Z]{2}[0-9]{2}(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])[HM][A-Z]{2}[B-DF-HJ-NP-TV-Z]{3}[A-Z0-9][0-9]$/;

/** Whether the CURP (trimmed, upper-cased) has the official shape the students service accepts. */
export function isCurpShape(raw: string): boolean {
  return CURP_SHAPE.test(raw.trim().toUpperCase());
}

const CHECKED_LENGTH = 17;
const CHECK_WEIGHT_BASE = 18;
const CHECK_MODULUS = 10;

/** The check digit RENAPO computes for a CURP's first seventeen characters. */
export function curpCheckDigit(first17: string): number {
  let sum = 0;
  [...first17].slice(0, CHECKED_LENGTH).forEach((ch, i) => {
    sum += CURP_ALPHABET.indexOf(ch) * (CHECK_WEIGHT_BASE - i);
  });
  return (CHECK_MODULUS - (sum % CHECK_MODULUS)) % CHECK_MODULUS;
}

/** The birth date a CURP carries, ISO; undefined when that day does not exist. */
function curpBirthDate(curp: string): string | undefined {
  const century = /[0-9]/.test(curp.charAt(16)) ? 1900 : 2000;
  const year = century + Number(curp.slice(4, 6));
  const month = Number(curp.slice(6, 8));
  const day = Number(curp.slice(8, 10));
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) return undefined;
  return date.toISOString().slice(0, 10);
}

/** An ISO day as the school writes it: dd/mm/aaaa. */
function schoolDay(iso: string): string {
  const [year, month, day] = iso.split("-");
  return `${day}/${month}/${year}`;
}

/**
 * The warnings a CURP deserves, in Spanish, given the birth date captured
 * beside it (ISO, or "" when none) and the file's sex (when known). A CURP
 * still being typed, or not of the official shape, gets none: its length and
 * format have their own messages.
 */
export function curpNotices(rawCurp: string, birthDate: string, sex: Sex = ""): string[] {
  const curp = rawCurp.trim().toUpperCase();
  if (!CURP_SHAPE.test(curp)) return [];
  const notices: string[] = [];
  if (curpCheckDigit(curp) !== Number(curp.charAt(CHECKED_LENGTH))) {
    notices.push("El último carácter del CURP no corresponde al resto (dígito verificador). Revísalo contra el acta; puedes guardar de todos modos.");
  }
  const carried = curpBirthDate(curp);
  if (carried === undefined) {
    notices.push(`La fecha dentro del CURP (${curp.slice(8, 10)}/${curp.slice(6, 8)}/${curp.slice(4, 6)}) no existe. Revísalo contra el acta.`);
  } else if (birthDate !== "" && carried !== birthDate) {
    notices.push(`El CURP dice que nació el ${schoolDay(carried)} y la fecha de nacimiento capturada es ${schoolDay(birthDate)}. Revisa cuál es la correcta.`);
  }
  const carriedSex = SEX_OF_CURP[curp.charAt(10)];
  if (carriedSex !== undefined && (sex === "F" || sex === "M") && sex !== carriedSex.sex) {
    notices.push(`El CURP es de ${carriedSex.word} y el sexo capturado no coincide. Revisa cuál es el correcto.`);
  }
  return notices;
}

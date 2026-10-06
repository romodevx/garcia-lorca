import type { Level } from "../../config/api";
import type { ImportRow, StudentImportResult } from "../../studentImport/api";

/** The school's levels, as Configuración lists them. */
export const LEVELS: Level[] = [
  { id: "l-pri", name: "Primaria", sepCode: "", enrollmentGranularity: "cycle" },
  { id: "l-bach", name: "Bachillerato", sepCode: "", enrollmentGranularity: "cycle" },
];

export function importRow(over: Partial<ImportRow>): ImportRow {
  return { row: 2, name: "PÉREZ LÓPEZ, ANA", curp: "PELA100101MDFRPNA1", destination: "Primaria · 1° · A", status: "new", ...over };
}

/** A Primaria file that matched, with one row of each kind. */
export const MATCHED: StudentImportResult = {
  cycle: { id: "c1", name: "2026–2027" },
  levels: [{ fileName: "Primaria", status: "matched", levelId: "l-pri", levelName: "Primaria", students: 4, noTuition: false }],
  placements: [
    { key: "Primaria|01|A", level: "Primaria", grade: "1°", group: "A", students: 2, capacity: 30, levelCreated: false, gradeCreated: false, groupCreated: true },
  ],
  rows: [
    importRow({ row: 2, status: "new" }),
    importRow({ row: 3, status: "warning", name: "RUIZ, BETO", curp: "RUXB100101HDFZXT02", warnings: ["phone_invalid", "email_invalid"] }),
    importRow({ row: 4, status: "already", name: "SOTO, CIRO", reason: "already_exists" }),
    importRow({ row: 5, status: "skipped", name: "LUNA, DANI", reason: "status_withdrawn" }),
  ],
  summary: { total: 4, new: 2, warnings: 1, already: 1, skipped: 1, pending: 0 },
  unresolved: [],
  planHash: "h1",
};

/** A Preparatoria file nobody has mapped yet; no tuition table either. */
export const UNRESOLVED: StudentImportResult = {
  ...MATCHED,
  levels: [{ fileName: "Preparatoria", status: "unresolved", students: 2, noTuition: false }],
  placements: [],
  rows: [importRow({ row: 2, status: "pending", reason: "level_unresolved", destination: "Preparatoria · 1° · A" }), importRow({ row: 3, status: "pending", reason: "level_unresolved" })],
  summary: { total: 2, new: 0, warnings: 0, already: 0, skipped: 0, pending: 2 },
  unresolved: ["Preparatoria"],
  planHash: "h2",
};

export const ALL_ALREADY: StudentImportResult = {
  ...MATCHED,
  rows: [importRow({ row: 2, status: "already", reason: "already_exists" }), importRow({ row: 3, status: "already", reason: "already_exists" })],
  summary: { total: 2, new: 0, warnings: 0, already: 2, skipped: 0, pending: 0 },
};

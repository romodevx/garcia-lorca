/**
 * What the preview and result screens show, computed from the gateway's answer.
 * Pure on purpose: every rule of the table (tabs, default tab, search, paging),
 * the tree of what gets created and the CSV report is tested here, and the
 * components stay thin.
 */
import { toCsv, type CsvColumn } from "../_lib/csv";
import { matchesSearch } from "../_design/textSearch";
import type { ImportPlacement, ImportRow, RowStatus, StudentImportResult } from "./api";
import { rowReasons } from "./copy";

export type ImportTab = "all" | "new" | "warning" | "already" | "skipped" | "pending";

/** In the order the tabs read. */
export const TABS: readonly { id: ImportTab; label: string }[] = [
  { id: "all", label: "Todos" },
  { id: "new", label: "Nuevos" },
  { id: "warning", label: "Con avisos" },
  { id: "already", label: "Ya estaban" },
  { id: "skipped", label: "No se importan" },
  { id: "pending", label: "Pendientes" },
];

export const STATUS_LABEL: Record<RowStatus, string> = {
  new: "Nuevo",
  warning: "Con avisos",
  already: "Ya estaba",
  skipped: "No se importa",
  pending: "Pendiente",
};

export const PAGE_SIZE = 50;

function inTab(r: ImportRow, tab: ImportTab): boolean {
  if (tab === "all") return true;
  // "Nuevos" is everything that goes in; "Con avisos" is a subset of it.
  if (tab === "new") return r.status === "new" || r.status === "warning";
  return r.status === tab;
}

export function tabCounts(rows: readonly ImportRow[]): Record<ImportTab, number> {
  const counts = { all: 0, new: 0, warning: 0, already: 0, skipped: 0, pending: 0 };
  for (const tab of TABS) counts[tab.id] = rows.filter((r) => inTab(r, tab.id)).length;
  return counts;
}

/** The first non-empty of No se importan → Con avisos → Todos: what needs eyes first. */
export function defaultTab(rows: readonly ImportRow[]): ImportTab {
  const counts = tabCounts(rows);
  if (counts.skipped > 0) return "skipped";
  if (counts.warning > 0) return "warning";
  return "all";
}

export function visibleRows(rows: readonly ImportRow[], tab: ImportTab, query: string): ImportRow[] {
  return rows.filter((r) => inTab(r, tab) && matchesSearch(query, r.name, r.curp));
}

export function pageOf<T>(rows: readonly T[], page: number): T[] {
  return rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
}

export interface TreeGrade {
  grade: string;
  groups: ImportPlacement[];
}

export interface TreeLevel {
  level: string;
  grades: TreeGrade[];
}

export interface CatalogTree {
  counts: { levels: number; grades: number; groups: number };
  levels: TreeLevel[];
}

/** "Se crearán 1 nivel, 6 grados, 12 grupos" and the nesting under it: only what is new. */
export function catalogTree(placements: readonly ImportPlacement[]): CatalogTree {
  const created = placements.filter((p) => p.levelCreated || p.gradeCreated || p.groupCreated);
  const levels: TreeLevel[] = [];
  const counts = { levels: 0, grades: 0, groups: 0 };
  for (const p of created) {
    let level = levels.find((l) => l.level === p.level);
    if (level === undefined) {
      level = { level: p.level, grades: [] };
      levels.push(level);
      if (p.levelCreated) counts.levels++;
    }
    let grade = level.grades.find((g) => g.grade === p.grade);
    if (grade === undefined) {
      grade = { grade: p.grade, groups: [] };
      level.grades.push(grade);
      if (p.gradeCreated) counts.grades++;
    }
    grade.groups.push(p);
    if (p.groupCreated) counts.groups++;
  }
  return { counts, levels };
}

/** The school's names for the levels this file goes to (the button and the result say them). */
export function destinationNames(result: Pick<StudentImportResult, "levels">): string[] {
  return result.levels.filter((l) => l.status !== "unresolved").map((l) => l.levelName ?? l.fileName);
}

/** Rows students never answered: the commit was cut halfway. */
export function notAnsweredCount(rows: readonly ImportRow[]): number {
  return rows.filter((r) => r.reason === "not_answered").length;
}

const REPORT_COLUMNS: readonly CsvColumn<ImportRow>[] = [
  { label: "# fila", value: (r) => String(r.row) },
  { label: "Alumno", value: (r) => r.name },
  { label: "CURP", value: (r) => r.curp },
  { label: "Destino", value: (r) => r.destination },
  { label: "Resultado", value: (r) => STATUS_LABEL[r.status] },
  { label: "Motivo", value: (r) => rowReasons(r).join("; ") },
  { label: "Matrícula", value: (r) => r.enrollmentNumber ?? "" },
];

/** The downloadable report: every row of the file, not just the visible page. */
export function reportCsv(rows: readonly ImportRow[]): string {
  return toCsv(REPORT_COLUMNS, rows);
}

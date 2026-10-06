import { describe, expect, it } from "vitest";

import type { ImportPlacement, ImportRow, StudentImportResult } from "./api";
import {
  PAGE_SIZE,
  catalogTree,
  defaultTab,
  destinationNames,
  notAnsweredCount,
  pageOf,
  reportCsv,
  tabCounts,
  visibleRows,
} from "./view";

function row(over: Partial<ImportRow>): ImportRow {
  return { row: 2, name: "PÉREZ LÓPEZ, ANA", curp: "PELA100101MDFRPNA1", destination: "Primaria · 1° · A", status: "new", ...over };
}

function placement(over: Partial<ImportPlacement>): ImportPlacement {
  return { key: "Primaria|01|A", level: "Primaria", grade: "1°", group: "A", students: 10, capacity: 30, levelCreated: false, gradeCreated: false, groupCreated: false, ...over };
}

const ROWS: ImportRow[] = [
  row({ row: 2, status: "new" }),
  row({ row: 3, status: "warning", warnings: ["phone_invalid"], name: "RUIZ, BETO", curp: "RUXB100101HDFZXT02" }),
  row({ row: 4, status: "already", reason: "already_exists" }),
  row({ row: 5, status: "skipped", reason: "status_withdrawn" }),
  row({ row: 6, status: "pending", reason: "level_unresolved" }),
];

describe("tabs", () => {
  it("counts each tab; Nuevos includes the ones with warnings", () => {
    expect(tabCounts(ROWS)).toEqual({ all: 5, new: 2, warning: 1, already: 1, skipped: 1, pending: 1 });
  });

  it("opens on the first non-empty of No se importan → Con avisos → Todos", () => {
    expect(defaultTab(ROWS)).toBe("skipped");
    expect(defaultTab(ROWS.filter((r) => r.status !== "skipped"))).toBe("warning");
    expect(defaultTab([row({ status: "new" })])).toBe("all");
  });
});

describe("visibleRows", () => {
  it("narrows by tab and by name or CURP, ignoring accents and case", () => {
    expect(visibleRows(ROWS, "new", "").map((r) => r.row)).toEqual([2, 3]);
    expect(visibleRows(ROWS, "all", "perez").map((r) => r.row)).toEqual([2, 4, 5, 6]);
    expect(visibleRows(ROWS, "all", "ruxb").map((r) => r.row)).toEqual([3]);
  });

  it("pages by 50", () => {
    const many = Array.from({ length: 120 }, (_, i) => row({ row: i + 2 }));
    expect(PAGE_SIZE).toBe(50);
    expect(pageOf(many, 0)).toHaveLength(50);
    expect(pageOf(many, 2).map((r) => r.row)).toEqual(Array.from({ length: 20 }, (_, i) => i + 102));
  });
});

describe("catalogTree", () => {
  it("counts and nests only what the import creates", () => {
    const tree = catalogTree([
      placement({ key: "Primaria|01|A", levelCreated: true, gradeCreated: true, groupCreated: true }),
      placement({ key: "Primaria|01|B", group: "B", levelCreated: true, gradeCreated: true, groupCreated: true }),
      placement({ key: "Primaria|02|A", grade: "2°", levelCreated: true, gradeCreated: true, groupCreated: true }),
      placement({ key: "Secundaria|01|A", level: "Secundaria" }),
    ]);

    expect(tree.counts).toEqual({ levels: 1, grades: 2, groups: 3 });
    expect(tree.levels).toEqual([
      {
        level: "Primaria",
        grades: [
          { grade: "1°", groups: [expect.objectContaining({ key: "Primaria|01|A" }), expect.objectContaining({ key: "Primaria|01|B" })] },
          { grade: "2°", groups: [expect.objectContaining({ key: "Primaria|02|A" })] },
        ],
      },
    ]);
  });

  it("is empty when everything already exists", () => {
    expect(catalogTree([placement({})]).counts).toEqual({ levels: 0, grades: 0, groups: 0 });
  });
});

describe("destinationNames", () => {
  it("lists the school's names for the levels the file goes to; an unresolved one by the file's name", () => {
    const result = {
      levels: [
        { fileName: "Preparatoria", status: "chosen", levelName: "Bachillerato", students: 30, noTuition: false },
        { fileName: "Primaria", status: "matched", levelName: "Primaria", students: 1, noTuition: false },
        { fileName: "Kinder", status: "create", students: 3, noTuition: true },
        { fileName: "Otro", status: "unresolved", students: 2, noTuition: false },
      ],
    } as Pick<StudentImportResult, "levels">;
    expect(destinationNames(result)).toEqual(["Bachillerato", "Primaria", "Kinder", "Otro"]);
  });
});

describe("notAnsweredCount", () => {
  it("counts the rows a cut commit left unanswered", () => {
    expect(notAnsweredCount([row({ status: "skipped", reason: "not_answered" }), row({})])).toBe(1);
  });
});

describe("reportCsv", () => {
  it("writes every row with result, reason and matrícula in Spanish", () => {
    const csv = reportCsv([
      row({ row: 2, status: "new", enrollmentNumber: "2026-00042" }),
      row({ row: 5, status: "skipped", reason: "status_withdrawn", warnings: ["phone_invalid"] }),
    ]);
    const lines = csv.split("\r\n");
    expect(lines[0]).toBe("# fila,Alumno,CURP,Destino,Resultado,Motivo,Matrícula");
    expect(lines[1]).toBe('2,"PÉREZ LÓPEZ, ANA",PELA100101MDFRPNA1,Primaria · 1° · A,Nuevo,,2026-00042');
    expect(lines[2]).toBe('5,"PÉREZ LÓPEZ, ANA",PELA100101MDFRPNA1,Primaria · 1° · A,No se importa,Baja en el sistema anterior; Teléfono no tiene 10 dígitos,');
  });
});

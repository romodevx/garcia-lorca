"use client";

import { useMemo, useState } from "react";

import type { FormatRule } from "../../_design/form/useFormatError";
import type { InlineColumn } from "../../_design/InlineCreateRow";
import type { CreateGradeLevelInput, CreateGroupInput, Cycle, GradeLevel, Group, Level } from "../../config/api";

/*
 * What creating a grade or a group means, whatever screen offers the row: the
 * proposed values, the checks, the sentences and the call. Shared by
 * GradeLevelsPanel / GroupsPanel and the «Estructura de la escuela» tree
 * (PLAN-redesign-estructura L2), so the rules and the words never drift apart.
 */

const NO_ORDINAL = "El orden del grado tiene que ser un número mayor que cero.";
const NO_CAPACITY = "El cupo del grupo tiene que ser un número mayor que cero.";
const GRADE_REFUSED = "No se pudo crear el grado. Revisa que no exista ya en este nivel con ese nombre o ese orden.";
const GROUP_REFUSED = "No se pudo crear el grupo. Revisa que no exista ya con ese nombre.";
const DEFAULT_CAPACITY = 30;
/** The last letter A + 25 reaches: the 27th group is proposed as Z too. */
const LAST_LETTER_OFFSET = 25;

/** Checked in the row at save (ISSUE-CC-104): said on the cell, not only below the table. */
const ORDINAL_RULES: Readonly<Record<string, FormatRule>> = {
  ordinal: (value) => (Number(value) >= 1 ? undefined : NO_ORDINAL),
};

/** Checked in the row at save (ISSUE-CC-104): cupo 0 + Enter used to say nothing there. */
const CAPACITY_RULES: Readonly<Record<string, FormatRule>> = {
  capacity: (value) => (Number(value) >= 1 ? undefined : NO_CAPACITY),
};

const GRADE_COLUMNS: InlineColumn[] = [
  { key: "name", label: "Nombre del grado", kind: "text", uppercase: true, required: true, help: "El grado dentro del nivel, como lo nombra el colegio." },
  { key: "ordinal", label: "Orden", kind: "number", required: true, width: 110 },
];

const GROUP_COLUMNS: InlineColumn[] = [
  { key: "name", label: "Nombre del grupo", kind: "text", uppercase: true, required: true, help: "La letra o el nombre que distingue este salón de los otros del mismo grado." },
  { key: "capacity", label: "Cupo", kind: "number", required: true, width: 140, help: "Cuántos alumnos caben. La asignación automática no mete a nadie de más." },
];

/** El orden que sigue, para no obligar a nadie a contar la lista. */
function nextOrdinal(gradeLevels: readonly GradeLevel[]): number {
  return gradeLevels.reduce((max, g) => Math.max(max, g.ordinal), 0) + 1;
}

/** La letra que sigue: A, B, C… (cuenta los grupos, no las letras ocupadas). */
function nextLetter(groups: readonly Group[]): string {
  return String.fromCharCode("A".charCodeAt(0) + Math.min(groups.length, LAST_LETTER_OFFSET));
}

/** Everything an `InlineCreateRow` needs, plus the sentence to show when it refuses. */
export interface CreateRow {
  columns: InlineColumn[];
  rules: Readonly<Record<string, FormatRule>>;
  /** Stable identity: the row watches it to refresh the cells nobody touched. */
  defaults: Record<string, string>;
  onCreate: (values: Record<string, string>) => Promise<void>;
  error: string | undefined;
}

export function useGradeCreateRow(level: Level, gradeLevels: readonly GradeLevel[], create: (input: CreateGradeLevelInput) => Promise<void>): CreateRow {
  const [error, setError] = useState<string | undefined>(undefined);
  const defaults = useMemo(() => ({ ordinal: String(nextOrdinal(gradeLevels)) }), [gradeLevels]);
  const onCreate = async (values: Record<string, string>): Promise<void> => {
    const ordinal = Number(values.ordinal);
    if (!Number.isFinite(ordinal) || ordinal < 1) {
      // Number("") es 0, y un 0 que nadie escribió es un dato falso. Se
      // relanza: para el renglón, "no pasó" es no pasó, sea por la red o por el dato.
      setError(NO_ORDINAL);
      throw new Error(NO_ORDINAL);
    }
    setError(undefined);
    try {
      await create({ levelId: level.id, name: (values.name ?? "").trim(), ordinal });
    } catch (cause) {
      // Se RELANZA: el renglón necesita saber que no se guardó para conservar lo escrito.
      setError(GRADE_REFUSED);
      throw cause;
    }
  };
  return { columns: GRADE_COLUMNS, rules: ORDINAL_RULES, defaults, onCreate, error };
}

/** `cycle` undefined: there is nothing to create groups in, and the row refuses (callers do not draw it then). */
export function useGroupCreateRow(cycle: Cycle | undefined, gradeLevel: GradeLevel, groups: readonly Group[], create: (input: CreateGroupInput) => Promise<void>): CreateRow {
  const [error, setError] = useState<string | undefined>(undefined);
  const defaults = useMemo(() => ({ name: nextLetter(groups), capacity: String(DEFAULT_CAPACITY) }), [groups]);
  const onCreate = async (values: Record<string, string>): Promise<void> => {
    const capacity = Number(values.capacity);
    if (!Number.isFinite(capacity) || capacity < 1) {
      // Un cupo 0 que nadie escribió no acepta alumnos: la asignación automática
      // se salta ese grupo en silencio. Se relanza, como arriba.
      setError(NO_CAPACITY);
      throw new Error(NO_CAPACITY);
    }
    if (cycle === undefined) throw new Error("no cycle to create the group in");
    setError(undefined);
    try {
      await create({ cycleId: cycle.id, gradeLevelId: gradeLevel.id, name: (values.name ?? "").trim(), capacity });
    } catch (cause) {
      setError(GROUP_REFUSED);
      throw cause;
    }
  };
  return { columns: GROUP_COLUMNS, rules: CAPACITY_RULES, defaults, onCreate, error };
}

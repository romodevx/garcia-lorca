"use client";

import type { ReactElement } from "react";

import { ChipRow } from "./StudentFilters";
import { chooseGrade, chooseGroup, chooseLevel, studentFacets, type FilterCatalog, type StudentFilterState } from "./studentFilter";

/** A chain choice: level → grade → group ("" = not chosen yet). */
export type GroupChoice = Pick<StudentFilterState, "levelId" | "gradeId" | "groupId">;

export const NO_GROUP_CHOICE: GroupChoice = { levelId: "", gradeId: "", groupId: "" };

const NO_PUPILS: readonly never[] = [];

/**
 * The T4 level → grade → group chips, chained, for picking ONE group (a
 * report) rather than narrowing a list of pupils: same rules and chips as
 * StudentFilters, without counts or search.
 */
export function GroupChain({ catalog, value, onChange }: { catalog: FilterCatalog; value: GroupChoice; onChange: (next: GroupChoice) => void }): ReactElement {
  const state: StudentFilterState = { ...value, q: "" };
  const facets = studentFacets(NO_PUPILS, catalog, state);
  const pick = (next: StudentFilterState): void => onChange({ levelId: next.levelId, gradeId: next.gradeId, groupId: next.groupId });
  return (
    <div style={{ display: "grid", gridTemplateColumns: "minmax(0, 1fr)", gap: 12 }}>
      <ChipRow label="Nivel" options={facets.levels} chosen={value.levelId} onChoose={(id) => pick(chooseLevel(state, id))} showCounts={false} />
      {facets.grades.length > 0 && <ChipRow label="Grado" options={facets.grades} chosen={value.gradeId} onChoose={(id) => pick(chooseGrade(state, id))} showCounts={false} />}
      {facets.groups.length > 0 && <ChipRow label="Grupo" options={facets.groups} chosen={value.groupId} onChoose={(id) => pick(chooseGroup(state, id))} showCounts={false} />}
    </div>
  );
}

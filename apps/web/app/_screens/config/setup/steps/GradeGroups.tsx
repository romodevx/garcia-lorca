"use client";

import type { ReactElement } from "react";

import type { GradeLevel, Group } from "../../../../config/api";
import { EmptyHint } from "../../parts";
import type { StatusRead, StepId } from "../setupSteps";
import { linkStyle } from "./Prerequisite";

/*
 * Under step 3's grades: the picked grade's groups, read-only, and the way to
 * step 6 where they are edited. Without it a grade picked here shows nothing,
 * and an imported school looks as if its groups were lost.
 */

export interface GradeGroupsProps {
  grade: GradeLevel;
  /** Every group of the cycle (the status read): nothing is drawn until it answers. */
  cycleGroups: StatusRead<Group[]>;
  onGo: (id: StepId) => void;
}

export function GradeGroups({ grade, cycleGroups, onGo }: GradeGroupsProps): ReactElement | null {
  if (cycleGroups.kind !== "ready") return null;
  const names = cycleGroups.value.filter((g) => g.gradeLevelId === grade.id).map((g) => g.name);
  const action = names.length === 0 ? "Crear grupos" : "Editar grupos";
  return (
    <div style={{ display: "grid", justifyItems: "start", gap: 4, marginTop: 16 }}>
      <EmptyHint>
        {names.length === 0 ? `${grade.name} aún no tiene grupos.` : `Grupos de ${grade.name}: ${names.join(" · ")}`}
      </EmptyHint>
      <button type="button" onClick={() => onGo("groups")} style={linkStyle} aria-label={`${action} de ${grade.name}`}>
        {action}
      </button>
    </div>
  );
}

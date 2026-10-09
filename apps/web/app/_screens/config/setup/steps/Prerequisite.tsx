"use client";

import type { ReactElement } from "react";

import type { Cycle, Level } from "../../../../config/api";
import { EmptyHint } from "../../parts";
import { STEPS, type StepId } from "../setupSteps";

/*
 * A step that cannot work yet says what comes first and links there (§4.3):
 * "Primero crea el ciclo escolar." + "Ir al paso 2".
 */

export type Need = "cycle" | "levels" | "grades";

const NEEDS: Record<Need, { text: string; step: StepId }> = {
  cycle: { text: "Primero crea el ciclo escolar.", step: "cycle" },
  levels: { text: "Primero agrega un nivel.", step: "levels" },
  grades: { text: "Primero agrega un grado.", step: "levels" },
};

/** What steps 4–6 need first: a cycle, then at least one level. */
export function needFor(cycle: Cycle | undefined, levels: readonly Level[]): Need | undefined {
  if (cycle === undefined) return "cycle";
  if (levels.length === 0) return "levels";
  return undefined;
}

const linkStyle = { minHeight: 44, color: "var(--forest)", fontWeight: 600, fontSize: 14.5, textDecoration: "underline", cursor: "pointer" } as const;

export function Prerequisite({ need, onGo }: { need: Need; onGo: (id: StepId) => void }): ReactElement {
  const { text, step } = NEEDS[need];
  const number = STEPS.find((s) => s.id === step)?.number;
  return (
    <div style={{ display: "grid", justifyItems: "start", gap: 4 }}>
      <EmptyHint>{text}</EmptyHint>
      <button type="button" onClick={() => onGo(step)} style={linkStyle}>Ir al paso {number}</button>
    </div>
  );
}

"use client";

import type { ReactElement } from "react";

/** One step: a stable id the screen switches on, and the words a person reads. */
export interface StepDef<Id extends string | number> {
  id: Id;
  label: string;
}

/**
 * The step's colour. `red` is for the irreversible (deleting a level);
 * `forest` for a flow that builds something (migrating pupils).
 */
export type StepperTone = "red" | "forest";

interface ToneStyle {
  border: string;
  bubble: string;
  ink: string;
}

const TONES: Record<StepperTone, ToneStyle> = {
  red: { border: "var(--red)", bubble: "var(--red)", ink: "#fff" },
  forest: { border: "var(--forest)", bubble: "var(--lime)", ink: "#0C3318" },
};

export interface StepperProps<Id extends string | number> {
  steps: readonly StepDef<Id>[];
  current: Id;
  tone: StepperTone;
}

/** "1 Subir · 2 Vista previa · 3 Resultado" — numbered pills that wrap at phone width. */
export function Stepper<Id extends string | number>({ steps, current, tone }: StepperProps<Id>): ReactElement {
  const colors = TONES[tone];
  return (
    <ol aria-label="Pasos" style={{ display: "flex", flexWrap: "wrap", gap: 8, listStyle: "none", padding: 0, margin: "0 0 18px" }}>
      {steps.map((step, index) => {
        const isCurrent = step.id === current;
        return (
          <li
            key={String(step.id)}
            aria-current={isCurrent ? "step" : undefined}
            style={{
              display: "inline-flex", alignItems: "center", gap: 8, padding: "6px 14px 6px 6px", borderRadius: "var(--r-pill)",
              fontSize: 14, fontWeight: 600, border: `1.5px solid ${isCurrent ? colors.border : "var(--border)"}`,
              background: isCurrent ? `color-mix(in oklab, ${colors.border} 10%, var(--surface))` : "var(--surface)",
              color: isCurrent ? "var(--text)" : "var(--text-muted)",
            }}
          >
            <span aria-hidden="true" style={{ width: 26, height: 26, borderRadius: 99, display: "grid", placeItems: "center", background: isCurrent ? colors.bubble : "var(--surface-3)", color: isCurrent ? colors.ink : "var(--text-muted)", fontSize: 13 }}>{index + 1}</span>
            {step.label}
          </li>
        );
      })}
    </ol>
  );
}

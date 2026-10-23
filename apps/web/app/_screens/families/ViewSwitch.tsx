"use client";

import type { ReactElement } from "react";

export type StudentsVista = "alumnos" | "familias";

const OPTIONS: readonly [StudentsVista, string][] = [["alumnos", "Alumnos"], ["familias", "Familias"]];

/**
 * "Alumnos | Familias" at the top of Estudiantes (ADR 0041 D1): two views of
 * the same pupils, not two modules. The same pill as the status tabs.
 */
export function ViewSwitch({ vista, onChange }: { vista: StudentsVista; onChange: (vista: StudentsVista) => void }): ReactElement {
  return (
    <div role="group" aria-label="Vista" style={{ display: "inline-flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)" }}>
      {OPTIONS.map(([id, label]) => {
        const on = vista === id;
        return (
          <button
            key={id}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(id)}
            style={{ height: 44, padding: "0 20px", borderRadius: 99, cursor: "pointer", background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 700, fontSize: 15, fontFamily: "var(--font-display)", boxShadow: on ? "var(--shadow-card)" : "none" }}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}

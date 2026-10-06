"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";

/**
 * The way into Migrar alumnos from Configuración, under the cycle it fills.
 * Rendered only when the flag is on and the caller holds students:import.
 */
export function StudentImportEntry({ onOpen }: { onOpen: () => void }): ReactElement {
  return (
    <section
      aria-label="Migrar alumnos del sistema anterior"
      style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 14, padding: "16px 20px", borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}
    >
      <div style={{ minWidth: 0 }}>
        <h3 style={{ fontSize: 17, fontWeight: 700, color: "var(--text)" }}>Migrar alumnos del sistema anterior</h3>
        <p style={{ fontSize: 14, color: "var(--text-muted)", marginTop: 2 }}>
          Sube el Excel de cada nivel: verás qué se crea antes de guardar nada.
        </p>
      </div>
      <Btn icon="upload" onClick={onOpen} style={{ height: 44 }}>Migrar alumnos</Btn>
    </section>
  );
}

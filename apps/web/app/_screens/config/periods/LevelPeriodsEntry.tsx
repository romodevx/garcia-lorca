"use client";

import type { ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import type { Level } from "../../../config/api";

/**
 * The way into a level's evaluation periods page from Configuración: each
 * grade's periods with their own dates are too many fields for this screen.
 */
export function LevelPeriodsEntry({ level, onOpen }: { level: Level; onOpen: (levelId: string) => void }): ReactElement {
  return (
    <section
      aria-label="Periodos de evaluación por grado"
      style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 14, padding: "16px 20px", borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}
    >
      <div style={{ minWidth: 0 }}>
        <h3 style={{ fontSize: 17, fontWeight: 700, color: "var(--text)" }}>Periodos de evaluación por grado</h3>
        <p style={{ fontSize: 14, color: "var(--text-muted)", marginTop: 2 }}>
          Fechas de inicio y fin de cada periodo, grado por grado. Se copian de un grado a otros en un clic.
        </p>
      </div>
      <Btn icon="calendar" onClick={() => onOpen(level.id)} aria-label={`Periodos por grado de ${level.name}`} style={{ height: 44 }}>
        Periodos por grado
      </Btn>
    </section>
  );
}

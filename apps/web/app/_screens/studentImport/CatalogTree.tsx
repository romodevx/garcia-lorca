"use client";

import { useState, type ReactElement } from "react";

import { NumberField } from "../../_design/form";
import type { ImportPlacement } from "../../studentImport/api";
import { pupils } from "../../studentImport/copy";
import { catalogTree } from "../../studentImport/view";
import { cardStyle } from "./parts";

const CAPACITY_ERROR = "El cupo es un número entero desde 1.";

function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

export interface CatalogTreeProps {
  placements: readonly ImportPlacement[];
  /** A new group's capacity changed: re-preview with it. */
  onCapacity: (key: string, capacity: number) => void;
}

/** "Se crearán 1 nivel, 6 grados, 12 grupos", collapsed; each new group's capacity editable. */
export function CatalogTree({ placements, onCapacity }: CatalogTreeProps): ReactElement | null {
  const tree = catalogTree(placements);
  if (tree.levels.length === 0) return null;
  const { levels, grades, groups } = tree.counts;
  return (
    <details style={cardStyle}>
      <summary style={{ cursor: "pointer", fontSize: 15.5, fontWeight: 700, color: "var(--text)", minHeight: 44, display: "flex", alignItems: "center" }}>
        {`Se crearán ${plural(levels, "nivel", "niveles")}, ${plural(grades, "grado", "grados")}, ${plural(groups, "grupo", "grupos")}`}
      </summary>
      <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 12 }}>
        {tree.levels.map((level) => level.grades.map((grade) => (
          <li key={`${level.level}|${grade.grade}`} style={{ display: "grid", gap: 8 }}>
            <p style={{ fontWeight: 600, color: "var(--text)", margin: 0 }}>{`${level.level} › ${grade.grade} › ${grade.groups.map((g) => g.group).join(", ")}`}</p>
            {grade.groups.filter((g) => g.groupCreated).map((g) => (
              <CapacityField key={g.key} placement={g} onCapacity={onCapacity} />
            ))}
          </li>
        )))}
      </ul>
    </details>
  );
}

function CapacityField({ placement, onCapacity }: { placement: ImportPlacement; onCapacity: CatalogTreeProps["onCapacity"] }): ReactElement {
  const [value, setValue] = useState(String(placement.capacity));
  const [error, setError] = useState<string | undefined>(undefined);
  const commit = (): void => {
    const n = Number(value);
    if (!Number.isInteger(n) || n < 1) {
      setError(CAPACITY_ERROR);
      return;
    }
    if (n !== placement.capacity) onCapacity(placement.key, n);
  };
  const over = placement.students > placement.capacity;
  return (
    <div style={{ maxWidth: 260 }}>
      <NumberField
        label={`Cupo de ${placement.level} ${placement.grade} ${placement.group}`}
        value={value}
        onChange={setValue}
        min={1}
        step={1}
        error={error}
        onClearError={() => setError(undefined)}
        onBlur={commit}
        help="Cuántos alumnos caben en el grupo nuevo. Por omisión, los que entran o 30, lo que sea mayor."
      />
      {over && <p style={{ fontSize: 13.5, color: "var(--warn-fg)", margin: "4px 0 0" }}>{`Sobrecupo: entran ${pupils(placement.students)}`}</p>}
    </div>
  );
}

"use client";

import type { ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import type { FamilyPupil } from "./familyPupil";

/** The pupils chosen so far, each with its own way out. */
export function PupilChips({ pupils, onRemove }: { pupils: readonly FamilyPupil[]; onRemove: (pupil: FamilyPupil) => void }): ReactElement | null {
  if (pupils.length === 0) return null;
  return (
    <ul aria-label="Alumnos elegidos" style={{ display: "flex", flexWrap: "wrap", gap: 8, listStyle: "none", margin: 0, padding: 0 }}>
      {pupils.map((p) => (
        <li key={p.id} style={{ display: "inline-flex", alignItems: "center", gap: 6, minHeight: 44, padding: "0 6px 0 14px", borderRadius: 99, background: "color-mix(in oklab, var(--forest) 12%, var(--surface))", border: "1.5px solid var(--forest)", color: "var(--text)", fontSize: 13.5, fontWeight: 600 }}>
          <span>{p.name}</span>
          <span style={{ fontWeight: 400, color: "var(--text-muted)" }}>{p.room}</span>
          <button type="button" aria-label={`Quitar a ${p.name}`} onClick={() => onRemove(p)} style={{ width: 36, height: 36, borderRadius: "50%", display: "grid", placeItems: "center", color: "var(--text-muted)", cursor: "pointer", background: "transparent" }}>
            <Icon name="close" size={15} />
          </button>
        </li>
      ))}
    </ul>
  );
}

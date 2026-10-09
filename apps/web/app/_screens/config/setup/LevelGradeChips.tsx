"use client";

import type { CSSProperties, ReactElement } from "react";

import type { GradeLevel, Level } from "../../../config/api";
import { SelectChip } from "../parts";

/*
 * The level (and grade) chips above steps 4–6. They read and write the ONE
 * selection the whole screen shares (`selectedLevelId`, `selectedGradeLevelId`),
 * so what was picked in step 3 is still picked here (PLAN §2).
 */

export interface LevelGradeChipsProps {
  levels: readonly Level[];
  selectedLevelId: string | undefined;
  onSelectLevel: (id: string) => void;
  /** The selected level's grades. Absent: the step picks a level only. */
  grades?: readonly GradeLevel[];
  selectedGradeId?: string | undefined;
  onSelectGrade?: (id: string) => void;
  /** A short count beside each grade ("1 grupo"), when it is known. */
  gradePill?: (gradeId: string) => string | undefined;
}

const rowStyle: CSSProperties = { display: "flex", flexWrap: "wrap", gap: 8 };
const legendStyle: CSSProperties = { fontSize: 13, fontWeight: 600, color: "var(--text-muted)", marginBottom: 6 };
const pillStyle: CSSProperties = {
  marginLeft: 4, // plus the spoken separator's space, ~8px as before
  padding: "2px 8px",
  borderRadius: "var(--r-pill)",
  background: "var(--surface-3)",
  color: "var(--text-muted)",
  fontSize: 12.5,
  fontWeight: 600,
};

export function LevelGradeChips({ levels, selectedLevelId, onSelectLevel, grades, selectedGradeId, onSelectGrade, gradePill }: LevelGradeChipsProps): ReactElement {
  return (
    <div style={{ display: "grid", gap: 14, marginBottom: 20 }}>
      <ChipGroup label="Nivel">
        {levels.map((l) => (
          <SelectChip key={l.id} selected={l.id === selectedLevelId} onClick={() => onSelectLevel(l.id)}>{l.name}</SelectChip>
        ))}
      </ChipGroup>
      {grades !== undefined && grades.length > 0 && onSelectGrade !== undefined && (
        <ChipGroup label="Grado">
          {[...grades].sort((a, b) => a.ordinal - b.ordinal).map((g) => (
            <SelectChip key={g.id} selected={g.id === selectedGradeId} onClick={() => onSelectGrade(g.id)}>
              {g.name}
              <Pill text={gradePill?.(g.id)} />
            </SelectChip>
          ))}
        </ChipGroup>
      )}
    </div>
  );
}

function ChipGroup({ label, children }: { label: string; children: ReactElement[] }): ReactElement {
  return (
    <div role="group" aria-label={label}>
      <p style={legendStyle}>{label}</p>
      <div style={rowStyle}>{children}</div>
    </div>
  );
}

/** Read by screen readers between the grade and its count, never seen ("1º, 1 grupo"). */
const spokenOnlyStyle: CSSProperties = { position: "absolute", width: 1, height: 1, overflow: "hidden", clipPath: "inset(50%)", whiteSpace: "nowrap" };

function Pill({ text }: { text: string | undefined }): ReactElement | null {
  if (text === undefined) return null;
  return (
    <>
      <span style={spokenOnlyStyle}>,</span>{" "}
      <span style={pillStyle}>{text}</span>
    </>
  );
}

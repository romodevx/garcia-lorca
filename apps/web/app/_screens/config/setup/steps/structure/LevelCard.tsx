"use client";

import { useId, type CSSProperties, type ReactElement } from "react";

import { InlineCreateRow } from "../../../../../_design/InlineCreateRow";
import type { CreateGradeLevelInput, CreateGroupInput, Cycle, GradeLevel, Group, Level } from "../../../../../config/api";
import type { NameCorrections } from "../../../CorrectableName";
import { useGradeCreateRow } from "../../../createRows";
import { MoveButton, type Direction } from "../../../LevelOrder";
import { DeletionStatus, useDeletionFeedback } from "../../../useDeletionFeedback";
import { EmptyHint, ErrorAlert } from "../../../parts";
import { GradeRow } from "./GradeRow";
import { levelSummary, settingsSummary } from "./summary";

export interface LevelCardProps {
  level: Level;
  /** This level's grades, by ordinal. */
  grades: readonly GradeLevel[];
  /** The selected cycle's groups by grade id; undefined while they cannot be told. */
  groupsByGrade: ReadonlyMap<string, readonly Group[]> | undefined;
  cycle: Cycle | undefined;
  createGrade: (input: CreateGradeLevelInput) => Promise<void>;
  createGroup: (input: CreateGroupInput) => Promise<void>;
  gradeCorrections: NameCorrections | undefined;
  groupCorrections: NameCorrections | undefined;
  /** config:write: Subir / Bajar. Undefined, no arrows. */
  move?: (direction: Direction) => void;
  isFirst: boolean;
  isLast: boolean;
  onOpenSettings: () => void;
}

/** Stable identity for a grade without groups (the create row memoizes on it). */
const NO_GROUPS: readonly Group[] = [];

const cardStyle: CSSProperties = {
  display: "grid",
  gap: 6,
  padding: "16px 18px",
  borderRadius: "var(--r-xl)",
  background: "var(--surface)",
  border: "1px solid var(--border)",
  boxShadow: "var(--shadow-card)",
};

const settingsButton: CSSProperties = {
  minHeight: 44,
  padding: "0 16px",
  borderRadius: "var(--r-pill)",
  border: "1px solid var(--border)",
  background: "transparent",
  color: "var(--text)",
  fontWeight: 600,
  fontSize: 14,
  cursor: "pointer",
};

/**
 * One level of «Estructura de la escuela»: its name and what it holds at a
 * glance, Subir / Bajar and «Ajustes» in its header, then a row per grade and
 * «Agregar grado a …» — the same create row as «Niveles y grados».
 */
export function LevelCard({ level, grades, groupsByGrade, cycle, createGrade, createGroup, gradeCorrections, groupCorrections, move, isFirst, isLast, onOpenSettings }: LevelCardProps): ReactElement {
  const titleId = useId();
  const feedback = useDeletionFeedback();
  const row = useGradeCreateRow(level, grades, createGrade);
  const groupsOf = (gradeId: string): readonly Group[] | undefined => (groupsByGrade === undefined ? undefined : groupsByGrade.get(gradeId) ?? NO_GROUPS);

  return (
    <section aria-labelledby={titleId} style={cardStyle}>
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: "6px 10px" }}>
        <div style={{ flex: "1 1 240px", minWidth: 0 }}>
          <h3 id={titleId} style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", margin: 0 }}>{level.name}</h3>
          <p style={{ margin: "2px 0 0", fontSize: 14, color: "var(--text-muted)" }}>{levelSummary(grades, groupsByGrade)}</p>
          <p style={{ margin: "2px 0 0", fontSize: 13.5, color: "var(--text-faint)" }}>{settingsSummary(level)}</p>
        </div>
        {move !== undefined && !isFirst && <MoveButton level={level} direction="up" onClick={() => move("up")} />}
        {move !== undefined && !isLast && <MoveButton level={level} direction="down" onClick={() => move("down")} />}
        <button type="button" aria-label={`Ajustes de ${level.name}`} onClick={onOpenSettings} style={settingsButton}>Ajustes</button>
      </div>

      {grades.length > 0 ? (
        <ul aria-label={`Grados de ${level.name}`} style={{ listStyle: "none", padding: 0, margin: 0 }}>
          {grades.map((g) => (
            <GradeRow
              key={g.id}
              grade={g}
              levelName={level.name}
              groups={groupsOf(g.id)}
              cycle={cycle}
              createGroup={createGroup}
              gradeCorrections={gradeCorrections}
              groupCorrections={groupCorrections}
              onGradeDeleted={feedback.deleted}
            />
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay grados en este nivel.</EmptyHint>
      )}

      <DeletionStatus message={feedback.message} />
      <div ref={feedback.fallbackRef} style={{ border: "1px solid var(--border)", borderRadius: "var(--r-md)", overflow: "hidden", marginTop: 4 }}>
        <InlineCreateRow addLabel={`Agregar grado a ${level.name}`} rules={row.rules} defaults={row.defaults} columns={row.columns} onCreate={row.onCreate} />
      </div>
      {row.error !== undefined && <ErrorAlert message={row.error} />}
    </section>
  );
}

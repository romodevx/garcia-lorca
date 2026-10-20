"use client";

import { useMemo, useState, type ReactElement } from "react";

import { Btn } from "../../../../../_design/Controls";
import { DrawerShell } from "../../../../../_design/DrawerShell";
import type { Cycle, GradeLevel, Group } from "../../../../../config/api";
import { LevelSettings, NewLevelDrawer } from "../../../LevelsPanel";
import { useLevelMoves } from "../../../LevelOrder";
import { EmptyHint, ErrorAlert } from "../../../parts";
import type { LevelsSlice } from "../../../useCatalogState";
import type { StatusRead } from "../../setupSteps";
import { StepFrame, type StepSlot } from "../../StepFrame";
import type { SetupActions } from "../../useSetupActions";
import type { RefreshKind } from "../../useSetupStatus";
import { Prerequisite } from "../Prerequisite";
import { LevelCard } from "./LevelCard";

export interface StepStructureProps {
  slot: StepSlot;
  cycle: Cycle | undefined;
  levels: LevelsSlice;
  /** Every grade of the school (the status read): the tree paints from it. */
  grades: StatusRead<GradeLevel[]>;
  /** Every group of the selected cycle (the status read). */
  groups: StatusRead<Group[]>;
  refresh: (kind: RefreshKind) => void;
  actions: SetupActions;
  /** config:write: Nuevo nivel and Subir / Bajar (CC-311, T8). */
  canWrite?: boolean;
  /** Given only to the owner (levels:delete). */
  onDeleteLevel?: (levelId: string) => void;
}

/**
 * Step 3 · Estructura de la escuela (module_school_structure,
 * PLAN-redesign-estructura variante A): every level as a card, every grade as a
 * row with its groups beside it — the whole school at a glance. The same
 * handlers and calls as «Niveles y grados» and «Grupos»; each level's
 * attendance, grading and deletion live in «Ajustes».
 */
export function StepStructure({ slot, cycle, levels, grades, groups, refresh, actions, canWrite = false, onDeleteLevel }: StepStructureProps): ReactElement {
  const [creating, setCreating] = useState(false);
  const [settingsOf, setSettingsOf] = useState<string | undefined>(undefined);
  const list = levels.list;
  const moves = useLevelMoves(list, levels.reorder);
  const settingsLevel = list.find((l) => l.id === settingsOf);

  // Grouped once per read, not in every row (L1).
  const byLevel = useMemo(() => {
    const out = new Map<string, GradeLevel[]>();
    if (grades.kind !== "ready") return out;
    for (const g of [...grades.value].sort((a, b) => a.ordinal - b.ordinal)) out.set(g.levelId, [...(out.get(g.levelId) ?? []), g]);
    return out;
  }, [grades]);
  const groupsRead = groups.kind === "ready" ? groups.value : undefined;
  const { structure } = actions;

  return (
    <StepFrame {...slot} ownHeading>
      <div style={{ display: "grid", gap: 14 }}>
        {canWrite && list.length > 0 && (
          <div><Btn kind="outline" icon="plus" onClick={() => setCreating(true)}>Nuevo nivel</Btn></div>
        )}
        {list.length === 0 && (
          <div style={{ display: "grid", gap: 10, justifyItems: "start" }}>
            <EmptyHint>Aún no hay niveles. Agrega el primero.</EmptyHint>
            {canWrite && <Btn icon="plus" onClick={() => setCreating(true)}>Nuevo nivel</Btn>}
          </div>
        )}
        {cycle === undefined && list.length > 0 && <Prerequisite need="cycle" onGo={slot.onGo} />}
        {grades.kind === "failed" && <Failed message="No se pudieron leer los grados." onRetry={() => refresh("grades")} />}
        {groups.kind === "failed" && cycle !== undefined && <Failed message="No se pudieron leer los grupos." onRetry={() => refresh("groups")} />}
        {moves.error !== undefined && <ErrorAlert message={moves.error} />}

        {grades.kind === "ready" && list.map((level, i) => (
          <LevelCard
            key={level.id}
            level={level}
            grades={byLevel.get(level.id) ?? []}
            groups={cycle === undefined ? undefined : groupsRead}
            cycle={cycle}
            createGrade={structure.createGrade}
            createGroup={structure.createGroup}
            gradeCorrections={structure.gradeCorrections}
            groupCorrections={structure.groupCorrections}
            move={canWrite ? (direction) => void moves.move(level, i, direction) : undefined}
            isFirst={i === 0}
            isLast={i === list.length - 1}
            onOpenSettings={() => setSettingsOf(level.id)}
          />
        ))}

        {canWrite && list.length > 2 && (
          <div><Btn kind="outline" icon="plus" onClick={() => setCreating(true)}>Nuevo nivel</Btn></div>
        )}
      </div>

      {creating && canWrite && <NewLevelDrawer onCreate={actions.createLevel} onClose={() => setCreating(false)} />}
      {settingsLevel !== undefined && (
        <DrawerShell title={`Ajustes de ${settingsLevel.name}`} subtitle="Asistencia y calificación del nivel" icon="settings" onClose={() => setSettingsOf(undefined)}>
          <LevelSettings level={settingsLevel} onSetAttendanceMode={levels.setAttendanceMode} onSetGrading={levels.setGrading} onDelete={onDeleteLevel} />
        </DrawerShell>
      )}
    </StepFrame>
  );
}

function Failed({ message, onRetry }: { message: string; onRetry: () => void }): ReactElement {
  return (
    <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 10 }}>
      <ErrorAlert message={message} />
      <Btn kind="ghost" onClick={onRetry}>Reintentar</Btn>
    </div>
  );
}

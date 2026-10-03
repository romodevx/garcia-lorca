"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { SelectField } from "../../_design/form";
import { GroupChain } from "../../_design/students/GroupChain";
import { Toast } from "../../_design/Toast";
import { UndoToast } from "../../_design/UndoToast";
import { useUndoableRemoval } from "../../_design/useUndoableRemoval";
import { API_URL } from "../../api-url";
import { deleteBlock, listUncovered, type ScheduleBoard, type ScheduleBlock } from "../../schedule/api";
import { filterBlocks, indexBoard, NO_WEEK_FILTER, type WeekFilter } from "../../schedule/catalog";
import { fridayOf, gridBounds, mondayOf, nowInSchoolZone } from "../../schedule/time";
import { BlockDrawer, type DrawerTarget } from "./BlockDrawer";
import { GridSettings } from "./GridSettings";
import { EmptyState } from "./parts";
import { WeekStepper } from "./UncoveredView";
import type { ScheduleMode } from "./useScheduleBoard";
import { WeekGrid } from "./WeekGrid";

const TOAST_MS = 3000;

export interface WeekViewProps {
  board: ScheduleBoard;
  mode: ScheduleMode;
  canWrite: boolean;
  now?: Date;
  /** Something was saved: read the board again. */
  onChanged: () => void;
  /** Bumped when an absence was saved: read the week's uncovered classes again. */
  version?: number;
}

/** The week: filters, the grid, the level's hours, and the drawer to add or change a class. */
export function WeekView({ board, mode, canWrite, now, onChanged, version = 0 }: WeekViewProps): ReactElement {
  const index = useMemo(() => indexBoard(board), [board]);
  const [filter, setFilter] = useState<WeekFilter>(() => ({ ...NO_WEEK_FILTER, levelId: mode === "manage" ? (board.levels[0]?.id ?? "") : "" }));
  const [target, setTarget] = useState<DrawerTarget | null>(null);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const removal = useUndoableRemoval<ScheduleBlock>();
  const today = nowInSchoolZone(now).date;
  const uncovered = useUncoveredWeek(board, mode, today, version);

  useEffect(() => {
    if (toast === undefined) return;
    const t = setTimeout(() => setToast(undefined), TOAST_MS);
    return () => clearTimeout(t);
  }, [toast]);

  const shown = filterBlocks(board, filter).filter((b) => !removal.pending.some((p) => p.id === b.id));
  const gridLevel = filter.levelId !== "" ? filter.levelId : index.levelOfGroup(shown[0]?.groupId ?? "");
  const grid = gridBounds(board.settings.find((s) => s.levelId === gridLevel), shown);
  const newClass = (weekday: number, start: number): void => setTarget({ kind: "new", weekday, start, groupId: filter.groupId, teacherId: filter.teacherId });

  const remove = (block: ScheduleBlock, reason: string): void => {
    setTarget(null);
    removal.remove(block, () => {
      deleteBlock(API_URL, block.id, reason === "" ? undefined : reason)
        .catch((err: unknown) => setToast(err instanceof Error ? err.message : "No se pudo quitar la clase."))
        .finally(onChanged);
    });
  };

  const saved = (message: string): void => {
    setTarget(null);
    setToast(message);
    onChanged();
  };

  return (
    <div style={{ display: "grid", gap: 16, gridTemplateColumns: "minmax(0, 1fr)" }}>
      {mode === "manage" && (
        <div style={{ display: "grid", gap: 12, gridTemplateColumns: "minmax(0, 1fr)" }}>
          <GroupChain catalog={board} value={filter} onChange={(next) => setFilter({ ...filter, ...next })} />
          <div style={{ display: "flex", gap: 14, alignItems: "flex-end", flexWrap: "wrap" }}>
            <div style={{ flex: "1 1 240px", maxWidth: 360 }}>
              <SelectField
                label="Docente"
                value={filter.teacherId}
                onChange={(teacherId) => setFilter({ ...filter, teacherId })}
                options={[...board.teachers]
                  .sort((a, b) => a.lastName.localeCompare(b.lastName, "es-MX"))
                  .map((t) => ({ value: t.id, label: `${t.firstName} ${t.lastName}` }))}
              />
            </div>
            {canWrite && filter.levelId !== "" && (
              <GridSettings key={filter.levelId} levelId={filter.levelId} settings={board.settings} onSaved={saved} />
            )}
            {canWrite && board.assignments.length > 0 && (
              <Btn icon="plus" onClick={() => newClass(1, grid.start)}>Agregar clase</Btn>
            )}
          </div>
        </div>
      )}

      <WeekEmptyState board={board} mode={mode} filter={filter} shownCount={shown.length} canWrite={canWrite} />

      {mode === "manage" && <WeekStepper monday={uncovered.monday} onMonday={uncovered.setMonday} />}
      <WeekGrid
        blocks={shown}
        uncovered={uncovered.blocks}
        index={index}
        grid={grid}
        onSlot={canWrite && board.assignments.length > 0 ? newClass : undefined}
        onOpen={canWrite ? (block) => setTarget({ kind: "edit", block }) : undefined}
      />

      {target !== null && (
        <BlockDrawer target={target} board={board} index={index} grid={grid} today={today} onClose={() => setTarget(null)} onSaved={saved} onRemove={remove} />
      )}
      {removal.latest !== undefined && <UndoToast message="Clase quitada" onUndo={() => removal.undo()} />}
      {toast !== undefined && removal.latest === undefined && <Toast>{toast}</Toast>}
    </div>
  );
}

/** Why the grid is empty, and what to do about it. */
function WeekEmptyState({ board, mode, filter, shownCount, canWrite }: { board: ScheduleBoard; mode: ScheduleMode; filter: WeekFilter; shownCount: number; canWrite: boolean }): ReactElement | null {
  if (mode === "teacher") return shownCount === 0 ? <EmptyState>Aún no tienes clases en el horario de este ciclo.</EmptyState> : null;
  if (board.assignments.length === 0) {
    return (
      <EmptyState>
        Este ciclo aún no tiene asignaciones de docentes. Una clase se arma con una de ellas: primero asigna a cada docente
        sus materias y grupos en Docentes (carga académica) y vuelve aquí.
      </EmptyState>
    );
  }
  if (shownCount > 0) return null;
  if (filter.groupId !== "") {
    return <EmptyState>{canWrite ? "Este grupo aún no tiene clases — haz clic en un hueco para agregar una." : "Este grupo aún no tiene clases."}</EmptyState>;
  }
  if (filter.teacherId !== "") return <EmptyState>Este docente aún no tiene clases en el horario.</EmptyState>;
  return <EmptyState>No hay clases con estos filtros. Elige un grupo para ver o armar su semana.</EmptyState>;
}

/**
 * The classes left uncovered by absences (T19) in the week on screen, by
 * block: the base week is the same every week, so the week chosen decides
 * which of its classes are painted "Sin cubrir". The teacher's own week reads
 * no absences (their "¿Qué está pasando ahora?" says "Ausente").
 */
function useUncoveredWeek(board: ScheduleBoard, mode: ScheduleMode, today: string, version: number): {
  monday: string;
  setMonday: (monday: string) => void;
  blocks: ReadonlySet<string>;
} {
  const [monday, setMonday] = useState(() => mondayOf(today));
  const [blocks, setBlocks] = useState<ReadonlySet<string>>(new Set());
  useEffect(() => {
    if (mode !== "manage") return;
    let live = true;
    listUncovered(API_URL, board.cycle.id, monday, fridayOf(monday))
      .then((got) => live && setBlocks(new Set(got.map((u) => u.class.blockId))))
      .catch(() => live && setBlocks(new Set()));
    return () => {
      live = false;
    };
  }, [board, mode, monday, version]);
  return { monday, setMonday, blocks };
}

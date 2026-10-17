"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { SelectField } from "../../_design/form";
import { GroupChain } from "../../_design/students/GroupChain";
import { Toast } from "../../_design/Toast";
import { UndoToast } from "../../_design/UndoToast";
import { useUndoableRemoval } from "../../_design/useUndoableRemoval";
import { byFullName, fullName } from "../../_lib/personName";
import { spanishServerReason } from "../../_lib/serverReason";
import { API_URL } from "../../api-url";
import { deleteBlock, type ScheduleBoard, type ScheduleBlock } from "../../schedule/api";
import { filterBlocks, indexBoard, NO_WEEK_FILTER, type WeekFilter } from "../../schedule/catalog";
import { gridBounds, nowInSchoolZone } from "../../schedule/time";
import { BlockDrawer, type DrawerTarget } from "./BlockDrawer";
import { GridSettings } from "./GridSettings";
import { WeekStepper } from "./UncoveredView";
import type { ScheduleMode } from "./useScheduleBoard";
import { useWeekChanges } from "./useWeekChanges";
import { WeekEmptyState } from "./WeekEmptyState";
import { WeekGrid } from "./WeekGrid";

const TOAST_MS = 3000;
const REMOVE_FAILED = "No se pudo quitar la clase.";

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
  // Only a school with ONE level starts on it: picking "the first" showed its
  // grades as if someone had chosen it (ISSUE-T14-016).
  const onlyLevel = board.levels.length === 1 ? board.levels[0]!.id : "";
  const [filter, setFilter] = useState<WeekFilter>(() => ({ ...NO_WEEK_FILTER, levelId: mode === "manage" ? onlyLevel : "" }));
  const [target, setTarget] = useState<DrawerTarget | null>(null);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const removal = useUndoableRemoval<ScheduleBlock>();
  const [gone, setGone] = useState<ReadonlySet<string>>(new Set());
  const today = nowInSchoolZone(now).date;
  const week = useWeekChanges(board, mode, today, version);

  useEffect(() => {
    if (toast === undefined) return;
    const t = setTimeout(() => setToast(undefined), TOAST_MS);
    return () => clearTimeout(t);
  }, [toast]);

  const shown = filterBlocks(board, filter).filter((b) => !gone.has(b.id) && !removal.pending.some((p) => p.id === b.id));
  const gridLevel = filter.levelId !== "" ? filter.levelId : index.levelOfGroup(shown[0]?.groupId ?? "");
  const grid = gridBounds(board.settings.find((s) => s.levelId === gridLevel), shown);
  const newClass = (weekday: number, start: number): void => setTarget({ kind: "new", weekday, start, groupId: filter.groupId, teacherId: filter.teacherId });

  const remove = (block: ScheduleBlock, reason: string): void => {
    setTarget(null);
    removal.remove(block, ({ keepalive }) => {
      // Hidden while sent: until the board is read again it still lists it, and
      // a class back on screen invites a second DELETE (gate #142). A refused
      // DELETE gives it back (gate B4).
      setGone((prev) => new Set(prev).add(block.id));
      deleteBlock(API_URL, block.id, reason === "" ? undefined : reason, { keepalive })
        .catch((err: unknown) => {
          setGone((prev) => {
            const next = new Set(prev);
            next.delete(block.id);
            return next;
          });
          setToast((err instanceof Error ? spanishServerReason(err.message) : undefined) ?? REMOVE_FAILED);
        })
        .finally(onChanged);
    });
  };

  const saved = (message: string): void => {
    removal.flush();
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
                options={byFullName(board.teachers).map((t) => ({ value: t.id, label: fullName(t) }))}
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

      {mode === "manage" && <WeekStepper monday={week.monday} onMonday={week.setMonday} />}
      <WeekGrid
        blocks={shown}
        uncovered={week.uncovered}
        substitutes={week.substitutes}
        index={index}
        grid={grid}
        onSlot={canWrite && board.assignments.length > 0 ? newClass : undefined}
        onOpen={canWrite ? (block) => setTarget({ kind: "edit", block }) : undefined}
      />

      {target !== null && (
        <BlockDrawer target={target} board={board} index={index} grid={grid} today={today} onClose={() => setTarget(null)} onSaved={saved} onRemove={remove} />
      )}
      {removal.latest !== undefined && <UndoToast message="Clase quitada" onUndo={() => removal.undo()} onDismiss={removal.dismiss} />}
      {toast !== undefined && removal.latest === undefined && <Toast>{toast}</Toast>}
    </div>
  );
}

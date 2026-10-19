import type { ReactElement } from "react";

import type { ScheduleBoard } from "../../schedule/api";
import type { WeekFilter } from "../../schedule/catalog";
import { EmptyState } from "./parts";
import type { ScheduleMode } from "./useScheduleBoard";

/** Why the grid is empty, and what to do about it. */
export function WeekEmptyState({ board, mode, filter, shownCount, canWrite }: { board: ScheduleBoard; mode: ScheduleMode; filter: WeekFilter; shownCount: number; canWrite: boolean }): ReactElement | null {
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

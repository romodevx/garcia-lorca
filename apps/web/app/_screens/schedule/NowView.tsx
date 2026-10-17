"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { DateField, SelectField } from "../../_design/form";
import { ChipRow } from "../../_design/students/StudentFilters";
import { Toast } from "../../_design/Toast";
import { API_URL } from "../../api-url";
import { deleteOverride, type ScheduleBoard, type ScheduleClass, type ScheduleDay } from "../../schedule/api";
import { indexBoard } from "../../schedule/catalog";
import { gridBounds, nowInSchoolZone, runningAt, timeOptions } from "../../schedule/time";
import { ClassCard, DayList } from "./ClassCard";
import { CoverDrawer } from "./CoverDrawer";
import { chipOption, EmptyState } from "./parts";
import type { ScheduleSource } from "./useScheduleBoard";

const TOAST_MS = 3000;

/**
 * The school day the hour picker spans (ISSUE-CC-122: it offered every five
 * minutes from 00:00): the chosen level's grid, or every level's together,
 * widened to the day's own classes the way the week grid is.
 */
function dayBounds(board: ScheduleBoard, levelId: string, classes: readonly ScheduleClass[]): { start: number; end: number } {
  const grids = board.settings.filter((s) => levelId === "" || s.levelId === levelId);
  const bounds = (grids.length > 0 ? grids : [undefined]).map((g) => gridBounds(g, classes));
  return { start: Math.min(...bounds.map((b) => b.start)), end: Math.max(...bounds.map((b) => b.end)) };
}

export interface NowViewProps {
  board: ScheduleBoard;
  source: ScheduleSource;
  canWrite: boolean;
  now?: Date;
}

/**
 * "¿Qué está pasando ahora?": the classes under way at one instant — today
 * and now at the school (America/Mexico_City) unless another date or time is
 * chosen — as cards that never overlap, and the whole day below with its
 * changes, each one undoable by a writer.
 */
export function NowView({ board, source, canWrite, now }: NowViewProps): ReactElement {
  const index = useMemo(() => indexBoard(board), [board]);
  const [start] = useState(() => nowInSchoolZone(now));
  const [date, setDate] = useState(start.date);
  const [time, setTime] = useState(start.time);
  const [levelId, setLevelId] = useState("");
  const [day, setDay] = useState<ScheduleDay | null>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [reads, setReads] = useState(0);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const [covering, setCovering] = useState<ScheduleClass | null>(null);
  const coverBlock = covering === null ? undefined : board.blocks.find((b) => b.id === covering.blockId);

  useEffect(() => {
    if (date === "") return;
    let live = true;
    source
      .day(board.cycle.id, date)
      .then((got) => {
        if (!live) return;
        setDay(got);
        setFailure(undefined);
      })
      .catch((err: unknown) => live && setFailure(err instanceof Error ? err.message : "No se pudo leer el día."));
    return () => {
      live = false;
    };
  }, [source, board.cycle.id, date, reads]);

  useEffect(() => {
    if (toast === undefined) return;
    const t = setTimeout(() => setToast(undefined), TOAST_MS);
    return () => clearTimeout(t);
  }, [toast]);

  const classes = (day?.classes ?? []).filter((c) => levelId === "" || index.levelOfGroup(c.groupId) === levelId);
  // A cancelled class still shows, struck through: "why is 3.º A free?" is the question.
  const running = runningAt(classes, time);
  // The exact minute ("08:07") stays choosable beside the 5-minute steps.
  const bounds = dayBounds(board, levelId, classes);
  const times = [...new Set([...timeOptions(bounds.start, bounds.end), time])].sort().map((t) => ({ value: t, label: t }));

  const undo = (overrideId: string): void => {
    deleteOverride(API_URL, overrideId)
      .then(() => {
        setToast("Cambio deshecho: la clase vuelve a su horario");
        setReads((n) => n + 1);
      })
      .catch((err: unknown) => setFailure(err instanceof Error ? err.message : "No se pudo deshacer el cambio."));
  };

  return (
    <div style={{ display: "grid", gap: 18, gridTemplateColumns: "minmax(0, 1fr)" }}>
      <div style={{ display: "flex", gap: 14, flexWrap: "wrap", alignItems: "flex-end" }}>
        <div style={{ width: 200 }}>
          <DateField label="Fecha" value={date} onChange={setDate} />
        </div>
        <div style={{ width: 130 }}>
          <SelectField label="Hora" value={time} onChange={(t) => t !== "" && setTime(t)} options={times} />
        </div>
      </div>
      {board.levels.length > 1 && (
        <ChipRow label="Nivel" options={board.levels.map((l) => chipOption(l.id, l.name))} chosen={levelId}
          onChoose={(id) => setLevelId(levelId === id ? "" : id)} showCounts={false} />
      )}

      {failure !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14.5 }}>{failure}</p>}

      <section aria-label={`Clases a las ${time}`} style={{ display: "grid", gap: 12 }}>
        <h2 style={{ fontSize: 18, fontWeight: 700, color: "var(--text)" }}>A las {time}</h2>
        {day !== null && day.nonWorking !== null && <EmptyState>Día inhábil: {day.nonWorking}. No hay clases.</EmptyState>}
        {day !== null && day.nonWorking === null && running.length === 0 && <EmptyState>No hay clases a esta hora</EmptyState>}
        {running.length > 0 && (
          // auto-fill: one column on a phone, two or three on a tablet, more on a monitor.
          <div style={{ display: "grid", gap: 14, gridTemplateColumns: "repeat(auto-fill, minmax(min(100%, 250px), 1fr))" }}>
            {running.map((c) => <ClassCard key={c.blockId} cls={c} index={index} onCover={canWrite ? setCovering : undefined} />)}
          </div>
        )}
      </section>

      {day !== null && classes.length > 0 && <DayList classes={classes} index={index} onUndo={canWrite ? undo : undefined} />}
      {covering !== null && coverBlock !== undefined && (
        <CoverDrawer board={board} index={index} block={coverBlock} date={date} absentTeacherId={covering.teacherId}
          replaceOverrideId={covering.overrideId} onClose={() => setCovering(null)}
          onSaved={(message) => { setCovering(null); setToast(message); setReads((n) => n + 1); }} />
      )}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </div>
  );
}

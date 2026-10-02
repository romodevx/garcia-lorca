"use client";

import { useCallback, useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { getBoard, getDay, getOwnBoard, getOwnDay, type ScheduleBoard, type ScheduleDay } from "../../schedule/api";

/** "manage": the school's timetable (schedule:read); "teacher": the caller's own (teaching:read). */
export type ScheduleMode = "manage" | "teacher";

/** Where a mode reads from: the teacher's own endpoints never take a cycle. */
export interface ScheduleSource {
  board: (cycleId?: string) => Promise<ScheduleBoard>;
  day: (cycleId: string, date: string) => Promise<ScheduleDay>;
}

const MANAGE: ScheduleSource = {
  board: (cycleId) => getBoard(API_URL, cycleId),
  day: (cycleId, date) => getDay(API_URL, cycleId, date),
};

const TEACHER: ScheduleSource = {
  board: () => getOwnBoard(API_URL),
  day: (_cycleId, date) => getOwnDay(API_URL, date),
};

export function sourceFor(mode: ScheduleMode): ScheduleSource {
  return mode === "manage" ? MANAGE : TEACHER;
}

export interface BoardState {
  board: ScheduleBoard | null;
  failure: string | undefined;
  /** Read the board again (after a save) without blanking the screen. */
  reload: () => void;
}

/** The board of `cycleId` (undefined: the active cycle), kept fresh across saves. */
export function useScheduleBoard(source: ScheduleSource, cycleId: string | undefined): BoardState {
  const [board, setBoard] = useState<ScheduleBoard | null>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [reads, setReads] = useState(0);

  useEffect(() => {
    let live = true;
    source
      .board(cycleId)
      .then((got) => {
        if (!live) return;
        setBoard(got);
        setFailure(undefined);
      })
      .catch((err: unknown) => {
        if (live) setFailure(err instanceof Error ? err.message : "No se pudo leer el horario.");
      });
    return () => {
      live = false;
    };
  }, [source, cycleId, reads]);

  const reload = useCallback(() => setReads((n) => n + 1), []);
  return { board, failure, reload };
}

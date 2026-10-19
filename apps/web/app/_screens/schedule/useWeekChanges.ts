"use client";

import { useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { getDay, listUncovered, type ScheduleBoard } from "../../schedule/api";
import { addDays, fridayOf, schoolWeekOf } from "../../schedule/time";
import type { ScheduleMode } from "./useScheduleBoard";

/** Monday to Friday. */
const SCHOOL_DAYS = 5;

export interface WeekChanges {
  monday: string;
  setMonday: (monday: string) => void;
  /** Blocks nobody gives that week (T19): painted "Sin cubrir". */
  uncovered: ReadonlySet<string>;
  /** Block id → the teacher covering it that week (ISSUE-CC-121). */
  substitutes: ReadonlyMap<string, string>;
}

/**
 * What changes the base week on the week on screen. The base week is the same
 * every week, so the week chosen decides which of its classes are painted
 * "Sin cubrir" and which carry a substitute. The teacher's own week reads
 * neither (their "¿Qué está pasando ahora?" says so).
 */
export function useWeekChanges(board: ScheduleBoard, mode: ScheduleMode, today: string, version: number): WeekChanges {
  const [monday, setMonday] = useState(() => schoolWeekOf(today));
  const [uncovered, setUncovered] = useState<ReadonlySet<string>>(new Set());
  const [substitutes, setSubstitutes] = useState<ReadonlyMap<string, string>>(new Map());

  useEffect(() => {
    if (mode !== "manage") return;
    let live = true;
    const cycleId = board.cycle.id;
    listUncovered(API_URL, cycleId, monday, fridayOf(monday))
      .then((got) => live && setUncovered(new Set(got.map((u) => u.class.blockId))))
      .catch(() => live && setUncovered(new Set()));
    // One read per school day: a substitute is a change of ONE date.
    const days = Array.from({ length: SCHOOL_DAYS }, (_, i) => addDays(monday, i));
    Promise.all(days.map((date) => getDay(API_URL, cycleId, date)))
      .then((read) => {
        if (!live) return;
        const covered = read.flatMap((d) => d.classes).filter((c) => c.status === "substitute");
        setSubstitutes(new Map(covered.map((c) => [c.blockId, c.teacherId])));
      })
      .catch(() => live && setSubstitutes(new Map()));
    return () => {
      live = false;
    };
  }, [board, mode, monday, version]);

  return { monday, setMonday, uncovered, substitutes };
}

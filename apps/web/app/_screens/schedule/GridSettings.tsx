"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { TimeField } from "../../_design/form";
import { API_URL } from "../../api-url";
import { putSettings, type ScheduleSettings } from "../../schedule/api";
import { DEFAULT_GRID, SCHOOL_DAY_EARLIEST, SCHOOL_DAY_LATEST, SLOT_MINUTES, timeError, toHHMM, toMinutes } from "../../schedule/time";

/** The contract's floor: a level's day lasts at least an hour. */
const MIN_GRID_MINUTES = 60;
const SHORT_DAY = "El día debe durar al menos una hora y terminar después de empezar.";

export interface GridSettingsProps {
  levelId: string;
  settings: readonly ScheduleSettings[];
  onSaved: (message: string) => void;
}

interface HourErrors {
  start?: string;
  end?: string;
}

/**
 * The hours the calendar shows for a level — two fields, so an inline row,
 * not a dialog. Existing classes are never touched: the grid widens itself
 * to show any class outside these hours.
 *
 * Each hour is typed (ISSUE-028), held to the school day (07:00–20:00); hours
 * already saved outside it are shown and kept as they are.
 */
export function GridSettings({ levelId, settings, onSaved }: GridSettingsProps): ReactElement {
  const current = settings.find((s) => s.levelId === levelId) ?? DEFAULT_GRID;
  const [gridStart, setGridStart] = useState<string>(current.gridStart);
  const [gridEnd, setGridEnd] = useState<string>(current.gridEnd);
  const [errors, setErrors] = useState<HourErrors>({});
  const [pending, setPending] = useState(false);

  const judge = (start: string, end: string): HourErrors => {
    const bounds = { min: SCHOOL_DAY_EARLIEST, max: SCHOOL_DAY_LATEST };
    const startError = timeError(start, { ...bounds, kept: current.gridStart });
    const endError = timeError(end, { ...bounds, kept: current.gridEnd });
    const short = toMinutes(end) - toMinutes(start) < MIN_GRID_MINUTES || start === "" || end === "" ? SHORT_DAY : undefined;
    return { start: startError, end: endError ?? (startError === undefined ? short : undefined) };
  };
  const shown = errors.start !== undefined || errors.end !== undefined;

  // Judged on blur; once an error shows, every change re-checks so it clears the moment it is fixed.
  const change = (start: string, end: string): void => {
    setGridStart(start);
    setGridEnd(end);
    if (shown) setErrors(judge(start, end));
  };

  const save = async (): Promise<void> => {
    const problems = judge(gridStart, gridEnd);
    setErrors(problems);
    if (problems.start !== undefined || problems.end !== undefined || pending) return;
    setPending(true);
    try {
      await putSettings(API_URL, levelId, { gridStart, gridEnd });
      onSaved(`Horas del nivel guardadas: ${gridStart} a ${gridEnd}`);
    } catch (err) {
      setErrors({ end: err instanceof Error ? err.message : "No se pudieron guardar las horas." });
    } finally {
      setPending(false);
    }
  };

  const bounds = { min: toHHMM(SCHOOL_DAY_EARLIEST), max: toHHMM(SCHOOL_DAY_LATEST), stepMinutes: SLOT_MINUTES };
  return (
    <div role="group" aria-label="Horas del nivel" style={{ display: "flex", gap: 10, alignItems: "flex-end", flexWrap: "wrap" }}>
      <div style={{ width: 140 }}>
        <TimeField label="Empieza" value={gridStart} error={errors.start} onChange={(v) => change(v, gridEnd)} onBlur={() => setErrors(judge(gridStart, gridEnd))} {...bounds} />
      </div>
      <div style={{ width: 140 }}>
        <TimeField label="Termina" value={gridEnd} error={errors.end} onChange={(v) => change(gridStart, v)} onBlur={() => setErrors(judge(gridStart, gridEnd))} {...bounds} />
      </div>
      <Btn kind="ghost" onClick={() => void save()}>{pending ? "Guardando…" : "Guardar horas"}</Btn>
    </div>
  );
}

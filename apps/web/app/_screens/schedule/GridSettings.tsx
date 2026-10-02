"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { SelectField } from "../../_design/form";
import { API_URL } from "../../api-url";
import { putSettings, type ScheduleSettings } from "../../schedule/api";
import { DEFAULT_GRID, timeOptions, toMinutes } from "../../schedule/time";

const MINUTES_PER_DAY = 24 * 60;
/** The contract's floor: a level's day lasts at least an hour. */
const MIN_GRID_MINUTES = 60;
const ALL_TIMES = timeOptions(0, MINUTES_PER_DAY).map((t) => ({ value: t, label: t }));

export interface GridSettingsProps {
  levelId: string;
  settings: readonly ScheduleSettings[];
  onSaved: (message: string) => void;
}

/**
 * The hours the calendar shows for a level — two fields, so an inline row,
 * not a dialog. Existing classes are never touched: the grid widens itself
 * to show any class outside these hours.
 */
export function GridSettings({ levelId, settings, onSaved }: GridSettingsProps): ReactElement {
  const current = settings.find((s) => s.levelId === levelId) ?? DEFAULT_GRID;
  const [gridStart, setGridStart] = useState<string>(current.gridStart);
  const [gridEnd, setGridEnd] = useState<string>(current.gridEnd);
  const [error, setError] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);

  const invalid = (start: string, end: string): string | undefined =>
    toMinutes(end) - toMinutes(start) < MIN_GRID_MINUTES ? "El día debe durar al menos una hora y terminar después de empezar." : undefined;

  // A select commits on change (there is no keystroke to forgive), so that is its "blur".
  const choose = (start: string, end: string): void => {
    setGridStart(start);
    setGridEnd(end);
    setError(invalid(start, end));
  };

  const save = async (): Promise<void> => {
    const problem = invalid(gridStart, gridEnd);
    setError(problem);
    if (problem !== undefined || pending) return;
    setPending(true);
    try {
      await putSettings(API_URL, levelId, { gridStart, gridEnd });
      onSaved(`Horas del nivel guardadas: ${gridStart} a ${gridEnd}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudieron guardar las horas.");
    } finally {
      setPending(false);
    }
  };

  return (
    <div role="group" aria-label="Horas del nivel" style={{ display: "flex", gap: 10, alignItems: "flex-end", flexWrap: "wrap" }}>
      <div style={{ width: 120 }}>
        <SelectField label="Empieza" value={gridStart} onChange={(v) => choose(v, gridEnd)} options={ALL_TIMES} />
      </div>
      <div style={{ width: 120 }}>
        <SelectField label="Termina" value={gridEnd} onChange={(v) => choose(gridStart, v)} error={error} options={ALL_TIMES} />
      </div>
      <Btn kind="ghost" onClick={() => void save()}>{pending ? "Guardando…" : "Guardar horas"}</Btn>
    </div>
  );
}

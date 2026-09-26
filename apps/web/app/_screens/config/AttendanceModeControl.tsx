"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { Toast } from "../../_design/Toast";
import type { AttendanceMode } from "../../attendance/sheet";
import type { Level } from "../../config/api";
import { ErrorAlert, RadioGroup } from "./parts";

const TOAST_MS = 2600;
const MODE_OPTIONS = [
  { value: "daily", label: "Por día" },
  { value: "per_class", label: "Por clase" },
] as const;
const MODE_TEXT: Record<AttendanceMode, string> = { daily: "por día", per_class: "por clase" };

export interface AttendanceModeControlProps {
  level: Level;
  onChange: (levelId: string, mode: AttendanceMode) => Promise<void>;
}

/**
 * How a level takes attendance, changed in one tap and saved at once: "Por
 * día" is one list per group each day; "Por clase" one per subject, taken by
 * that subject's teacher. A level that does not say is read as "Por día".
 */
export function AttendanceModeControl({ level, onChange }: AttendanceModeControlProps): ReactElement {
  const [error, setError] = useState<string | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const timer = useRef<number | undefined>(undefined);
  const current: AttendanceMode = level.attendanceMode ?? "daily";

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const change = async (mode: AttendanceMode): Promise<void> => {
    if (mode === current) return;
    setError(undefined);
    try {
      await onChange(level.id, mode);
      window.clearTimeout(timer.current);
      setToast(`${level.name}: asistencia ${MODE_TEXT[mode]}`);
      timer.current = window.setTimeout(() => setToast(undefined), TOAST_MS);
    } catch {
      setError("No se pudo cambiar la asistencia del nivel. Reintenta.");
    }
  };

  return (
    <div style={{ display: "grid", gap: 6, margin: "0 0 16px" }}>
      <RadioGroup legend={`Asistencia de ${level.name}`} options={MODE_OPTIONS} value={current} onChange={(mode) => void change(mode)} />
      <p style={{ margin: 0, fontSize: 13, color: "var(--text-muted)" }}>
        Por día: una lista diaria por grupo. Por clase: una lista por asignatura, que toma su maestro.
      </p>
      {error !== undefined && <ErrorAlert message={error} />}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </div>
  );
}

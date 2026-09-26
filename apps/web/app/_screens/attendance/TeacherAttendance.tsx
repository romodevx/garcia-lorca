"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { Page } from "../../_design/Page";
import {
  getTeachingAttendanceSheet,
  listAttendanceClasses,
  saveTeachingAttendance,
  type AttendanceClass,
} from "../../teaching/api";
import { ErrorAlert } from "../config/parts";
import { TeachingPicker } from "../teaching/TeachingPicker";
import { subjectLabel } from "../teaching/selection";
import { AttendanceHeader } from "./AttendanceHeader";
import { toAttendanceOptions, type AttendanceOption } from "./attendanceOptions";
import { PassSheet } from "./PassSheet";
import { todayISO } from "./sheetModel";
import { useLeaveGuard } from "./useLeaveGuard";

export interface TeacherAttendanceProps {
  onBack: () => void;
  /** Pins the day (tests, deep links); defaults to the local today. */
  today?: string;
}

const titleOf = (option: AttendanceOption): string =>
  option.attendanceSubjectId !== undefined ? `${option.groupName} · ${subjectLabel(option)}` : option.groupName;

/**
 * Teacher-scoped pass list (module_attendance, `attendance_own:read/write`).
 * It talks ONLY to the `/api/v1/teaching/*` client, so a teacher never
 * triggers a students-list, admin `/attendance/*` or `/config/*` fetch.
 *
 * What the teacher picks depends on each level's mode: daily groups are picked
 * by group; per-class levels by subject first (with "Lista del día" beside the
 * subjects when the teacher also has daily groups). Steps with one choice are
 * skipped. The gateway is the security boundary — this is UX only.
 */
export function TeacherAttendance({ onBack, today }: TeacherAttendanceProps): ReactElement {
  const [classes, setClasses] = useState<AttendanceClass[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const guard = useLeaveGuard();
  const day = today ?? todayISO();

  useEffect(() => {
    let active = true;
    listAttendanceClasses()
      .then((cs) => active && setClasses(cs))
      .catch(() => active && setLoadError("No se pudieron cargar tus grupos."))
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const { options, bySubject } = useMemo(() => toAttendanceOptions(classes), [classes]);

  return (
    <Page>
      <AttendanceHeader subtitle="Pase de lista · solo tus grupos" onBack={() => guard.canLeave() && onBack()} />
      <div className="fade-up">
        {loading ? (
          <p style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>
        ) : loadError !== undefined ? (
          <ErrorAlert message={loadError} />
        ) : (
          <TeachingPicker title="Asistencia" options={options} bySubject={bySubject} keyByGroup canLeave={guard.canLeave}>
            {(option) => (
              <PassSheet
                key={option.id}
                title={titleOf(option)}
                groupId={option.groupId}
                subjectId={option.attendanceSubjectId}
                today={day}
                restrictDates
                load={getTeachingAttendanceSheet}
                save={saveTeachingAttendance}
                onDirtyChange={guard.setDirty}
              />
            )}
          </TeachingPicker>
        )}
      </div>
    </Page>
  );
}

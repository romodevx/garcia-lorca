"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { listGroups, type TeacherGroup } from "../../teaching/api";
import { ErrorAlert } from "../config/parts";
import { TeachingPicker } from "../teaching/TeachingPicker";
import type { TeachingOption } from "../teaching/selection";
import { TeacherPassList } from "./TeacherPassList";

export interface TeacherAttendanceProps {
  onBack: () => void;
  /** Pins the day (tests, deep links); defaults to the local today. */
  today?: string;
}

/** Local calendar day as `YYYY-MM-DD` (the picker's and the gateway's format). */
function todayISO(): string {
  const now = new Date();
  const month = `${now.getMonth() + 1}`.padStart(2, "0");
  const day = `${now.getDate()}`.padStart(2, "0");
  return `${now.getFullYear()}-${month}-${day}`;
}

/**
 * Teacher-scoped daily pass list (module_attendance, `attendance_own:read/write`).
 * The self-scoped sibling of the director's screen: it talks ONLY to the
 * `/api/v1/teaching/*` client, so a teacher never triggers a students-list,
 * admin `/attendance/*`, or `/config/*` fetch (all of which they lack
 * permission for).
 *
 * Fewest taps: attendance is per GROUP today, so the picker has a single step,
 * skipped when the teacher has one group; the list then opens on today. The
 * gateway is the security boundary — this is UX only.
 */
export function TeacherAttendance({ onBack, today }: TeacherAttendanceProps): ReactElement {
  const [groups, setGroups] = useState<TeacherGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const day = today ?? todayISO();

  // Boot: the teacher's own groups.
  useEffect(() => {
    let active = true;
    setLoading(true);
    listGroups()
      .then((gs) => active && setGroups(gs))
      .catch(() => active && setLoadError("No se pudieron cargar tus grupos."))
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const options = useMemo<(TeachingOption & { group: TeacherGroup })[]>(
    () => groups.map((g) => ({ id: g.id, groupId: g.id, groupName: g.name, group: g })),
    [groups],
  );

  return (
    <Page>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, minHeight: 44, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 6, cursor: "pointer" }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 20 }}>
        <div style={{ width: 56, height: 56, flexShrink: 0, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--lime)", color: "var(--primary-ink)", boxShadow: "0 6px 16px color-mix(in oklab, var(--lime) 40%, transparent)" }}>
          <Icon name="qr" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Asistencia</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Pase de lista diario · solo tus grupos</p>
        </div>
      </div>

      <div className="fade-up">
        {loading ? (
          <p style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>
        ) : loadError !== undefined ? (
          <ErrorAlert message={loadError} />
        ) : (
          <TeachingPicker title="Asistencia" options={options} bySubject={false}>
            {(option) => <TeacherPassList key={option.id} group={option.group} today={day} />}
          </TeachingPicker>
        )}
      </div>
    </Page>
  );
}

"use client";

import type { CSSProperties, ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Page } from "../../_design/Page";
import { getAttendanceSheet, saveAttendanceSheet, scanAttendanceCard } from "../../attendance/api";
import type { ScanCardInput } from "../../attendance/scan";
import type { AttendanceSheetQuery, SaveSheetInput } from "../../attendance/sheet";
import { EmptyHint, ErrorAlert, SelectChip } from "../config/parts";
import { AttendanceHeader } from "./AttendanceHeader";
import { PassSheet } from "./PassSheet";
import { todayISO } from "./sheetModel";
import { useDirectorClasses, type DirectorClasses } from "./useDirectorClasses";
import { useLeaveGuard } from "./useLeaveGuard";

export interface DirectorAttendanceProps {
  onBack: () => void;
  /** Pins the day (tests, deep links); defaults to the local today. */
  today?: string;
  /** module_student_card is on: the list offers "Escanear QR". */
  canScanCards?: boolean;
}

const SELECT: CSSProperties = { minHeight: 48, maxWidth: "100%", padding: "0 16px", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 16, fontFamily: "var(--font-ui)", cursor: "pointer" };

const loadSheet = (q: AttendanceSheetQuery) => getAttendanceSheet(API_URL, q);
const saveSheet = (input: SaveSheetInput) => saveAttendanceSheet(API_URL, input);
const scanCard = (input: ScanCardInput) => scanAttendanceCard(API_URL, input);

function SubjectStep({ classes, canLeave }: { classes: DirectorClasses; canLeave: () => boolean }): ReactElement {
  if (classes.subjectsLoading) return <EmptyHint>Cargando asignaturas…</EmptyHint>;
  if (classes.subjects.length === 0) {
    return <EmptyHint>Este grupo toma asistencia por clase y aún no tiene asignaturas asignadas. Asígnalas en Docentes → Carga académica.</EmptyHint>;
  }
  return (
    <section style={{ marginBottom: 16 }}>
      <h2 style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", margin: "0 0 10px" }}>Elige la asignatura</h2>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
        {classes.subjects.map((s) => (
          <SelectChip key={s.id} selected={s.id === classes.subjectId} onClick={() => s.id !== classes.subjectId && canLeave() && classes.setSubjectId(s.id)}>{s.name}</SelectChip>
        ))}
      </div>
    </section>
  );
}

/**
 * School-wide pass list (attendance:read / attendance:write): any group of any
 * cycle, any date. A group whose level takes attendance per class asks for the
 * subject first. The list itself is the same PassSheet the teacher uses.
 */
export function DirectorAttendance({ onBack, today, canScanCards = false }: DirectorAttendanceProps): ReactElement {
  const classes = useDirectorClasses();
  const guard = useLeaveGuard();
  const { group, mode, subjectId } = classes;
  const subjectName = classes.subjects.find((s) => s.id === subjectId)?.name;
  const ready = group !== undefined && (mode === "daily" || subjectId !== "");

  return (
    <Page>
      <AttendanceHeader subtitle="Pase de lista · todos presentes, marca solo las excepciones" onBack={() => guard.canLeave() && onBack()} />
      {classes.loadError !== undefined ? (
        <ErrorAlert message={classes.loadError} />
      ) : (
        <div className="fade-up">
          <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 16 }}>
            <select aria-label="Ciclo" value={classes.cycleId} onChange={(e) => guard.canLeave() && classes.setCycleId(e.target.value)} style={SELECT}>
              {classes.cycles.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
            <select aria-label="Grupo" value={classes.groupId} onChange={(e) => guard.canLeave() && classes.setGroupId(e.target.value)} style={SELECT}>
              <option value="">Selecciona un grupo…</option>
              {classes.groups.map((g) => <option key={g.id} value={g.id}>{g.label}</option>)}
            </select>
          </div>
          {group === undefined && <EmptyHint>Selecciona un grupo para pasar lista.</EmptyHint>}
          {group !== undefined && mode === "per_class" && <SubjectStep classes={classes} canLeave={guard.canLeave} />}
          {ready && (
            <PassSheet
              key={`${group.id}:${subjectId}`}
              title={subjectName !== undefined ? `${group.label} · ${subjectName}` : group.label}
              groupId={group.id}
              subjectId={mode === "per_class" ? subjectId : undefined}
              today={today ?? todayISO()}
              restrictDates={false}
              load={loadSheet}
              save={saveSheet}
              scan={canScanCards ? scanCard : undefined}
              onDirtyChange={guard.setDirty}
            />
          )}
        </div>
      )}
    </Page>
  );
}

"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { ConfirmDialog } from "../../_design/ConfirmDialog";
import { Btn } from "../../_design/Controls";
import { SelectField } from "../../_design/form";
import { ChipRow } from "../../_design/students/StudentFilters";
import { Toast } from "../../_design/Toast";
import { API_URL } from "../../api-url";
import { cancelAbsence, listAbsences, listUncovered, type ScheduleBoard, type TeacherAbsence, type UncoveredClass } from "../../schedule/api";
import { indexBoard, type ScheduleIndex } from "../../schedule/catalog";
import { addDays, fridayOf, schoolWeekOf, spanishDate, weekLabel } from "../../schedule/time";
import { ABSENCE_KIND_LABEL } from "./AbsenceDrawer";
import { CoverDrawer } from "./CoverDrawer";
import { card, chipOption, EmptyState, StatusBadge } from "./parts";

const TOAST_MS = 3000;
const WEEK = 7;

/** "today": one date; "week": Monday–Friday of `monday`. */
export type UncoveredRange = { kind: "today" } | { kind: "week"; monday: string };

/**
 * Where "Cubrir ahora" lands for an absence: today when it is exactly today,
 * otherwise the school week of its first date — past or future, the classes
 * the notice just counted are the ones on screen.
 */
export function rangeForAbsence(a: { startsOn: string; endsOn: string }, today: string): UncoveredRange {
  return a.startsOn === today && a.endsOn === today ? { kind: "today" } : { kind: "week", monday: schoolWeekOf(a.startsOn) };
}

export interface UncoveredViewProps {
  board: ScheduleBoard;
  canWrite: boolean;
  /** The school's date (aaaa-mm-dd). */
  today: string;
  range: UncoveredRange;
  onRange: (range: UncoveredRange) => void;
  /** Bumped when an absence was saved elsewhere: read again. */
  version: number;
  onEditAbsence: (absence: TeacherAbsence) => void;
}

/** "del lunes 5 de octubre al miércoles 7 de octubre, de 08:00 a 10:00". */
export function absenceWhen(a: TeacherAbsence): string {
  const dates = a.startsOn === a.endsOn ? spanishDate(a.startsOn) : `del ${spanishDate(a.startsOn)} al ${spanishDate(a.endsOn)}`;
  return `${dates}, ${a.allDay ? "todo el día" : `de ${a.startsAt ?? ""} a ${a.endsAt ?? ""}`}`;
}

/** "Ana Ruiz · Ausente" — and why, only for whoever may read it (the gateway decides). */
export function absentLine(index: ScheduleIndex, teacherId: string, absence: TeacherAbsence | null): string {
  const kind = absence?.kind;
  return `${index.teacherName(teacherId)} · Ausente${kind !== undefined ? ` (${ABSENCE_KIND_LABEL[kind]})` : ""}`;
}

/**
 * "Clases por cubrir" (T19): today's or a week's classes nobody gives because
 * their teacher is absent, each one a click away from a substitute (the T16
 * suggestions, absent teachers left out), filterable by level and by the
 * absent teacher; below, the absences of those dates, to edit or cancel.
 */
export function UncoveredView({ board, canWrite, today, range, onRange, version, onEditAbsence }: UncoveredViewProps): ReactElement {
  const index = useMemo(() => indexBoard(board), [board]);
  const from = range.kind === "today" ? today : range.monday;
  const to = range.kind === "today" ? today : fridayOf(range.monday);
  const [classes, setClasses] = useState<UncoveredClass[] | null>(null);
  const [absences, setAbsences] = useState<TeacherAbsence[]>([]);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [reads, setReads] = useState(0);
  const [levelId, setLevelId] = useState("");
  const [teacherId, setTeacherId] = useState("");
  const [covering, setCovering] = useState<UncoveredClass | null>(null);
  const [cancelling, setCancelling] = useState<TeacherAbsence | null>(null);
  const [toast, setToast] = useState<string | undefined>(undefined);

  useEffect(() => {
    let live = true;
    Promise.all([listUncovered(API_URL, board.cycle.id, from, to), listAbsences(API_URL, { cycleId: board.cycle.id, from, to })])
      .then(([got, abs]) => {
        if (!live) return;
        setClasses(got);
        setAbsences(abs);
        setFailure(undefined);
      })
      .catch((err: unknown) => live && setFailure(err instanceof Error ? err.message : "No se pudieron leer las clases por cubrir."));
    return () => {
      live = false;
    };
  }, [board.cycle.id, from, to, reads, version]);

  useEffect(() => {
    if (toast === undefined) return;
    const t = setTimeout(() => setToast(undefined), TOAST_MS);
    return () => clearTimeout(t);
  }, [toast]);

  const shown = (classes ?? []).filter(
    (u) => (levelId === "" || index.levelOfGroup(u.class.groupId) === levelId) && (teacherId === "" || u.class.teacherId === teacherId),
  );
  const absentTeachers = [...new Set((classes ?? []).map((u) => u.class.teacherId))]
    .map((id) => ({ value: id, label: index.teacherName(id) }))
    .sort((a, b) => a.label.localeCompare(b.label, "es-MX"));
  const coverBlock = covering === null ? undefined : board.blocks.find((b) => b.id === covering.class.blockId);
  const reread = (message: string): void => {
    setCovering(null);
    setCancelling(null);
    setToast(message);
    setReads((n) => n + 1);
  };

  const cancel = (a: TeacherAbsence): void => {
    cancelAbsence(API_URL, a.id)
      .then(() => reread("Ausencia cancelada: sus clases vuelven a su horario"))
      .catch((err: unknown) => {
        setCancelling(null);
        setFailure(err instanceof Error ? err.message : "No se pudo cancelar la ausencia.");
      });
  };

  return (
    <div style={{ display: "grid", gap: 18, gridTemplateColumns: "minmax(0, 1fr)" }}>
      <RangePicker range={range} onRange={onRange} today={today} />
      <div style={{ display: "flex", gap: 14, flexWrap: "wrap", alignItems: "flex-end" }}>
        {board.levels.length > 1 && (
          <ChipRow label="Nivel" options={board.levels.map((l) => chipOption(l.id, l.name))} chosen={levelId}
            onChoose={(id) => setLevelId(levelId === id ? "" : id)} showCounts={false} />
        )}
        <div style={{ flex: "1 1 220px", maxWidth: 320 }}>
          <SelectField label="Docente ausente" value={teacherId} onChange={setTeacherId} options={absentTeachers} />
        </div>
      </div>

      {failure !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14.5 }}>{failure}</p>}

      {classes !== null && shown.length === 0 && (
        <EmptyState>{range.kind === "today" ? "No hay clases por cubrir hoy." : "No hay clases por cubrir esa semana."}</EmptyState>
      )}
      {shown.length > 0 && (
        <ul aria-label="Clases por cubrir" style={{ ...card, listStyle: "none", margin: 0, padding: "4px 0" }}>
          {shown.map((u) => (
            <li key={`${u.date}-${u.class.blockId}`} style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", padding: "12px 20px", borderTop: "1px solid var(--border)" }}>
              <span style={{ minWidth: 150, color: "var(--text-muted)", fontSize: 14 }}>
                {spanishDate(u.date)}<br /><span style={{ fontVariantNumeric: "tabular-nums" }}>{u.class.startsAt}–{u.class.endsAt}</span>
              </span>
              <span style={{ fontWeight: 700, minWidth: 64 }}>{index.groupLabel(u.class.groupId)}</span>
              <span style={{ flex: "1 1 220px", minWidth: 0 }}>
                {index.subjectName(u.class.subjectId)} · {absentLine(index, u.class.teacherId, u.class.absence)}
              </span>
              <StatusBadge status="uncovered" />
              {canWrite && (
                <Btn kind="outline" onClick={() => setCovering(u)} style={{ height: 44, fontSize: 14 }}
                  aria-label={`Asignar suplente a ${index.subjectName(u.class.subjectId)}, ${index.groupLabel(u.class.groupId)}, ${spanishDate(u.date)} ${u.class.startsAt}`}>
                  Asignar suplente
                </Btn>
              )}
            </li>
          ))}
        </ul>
      )}

      <RegisteredAbsences absences={absences} index={index} canWrite={canWrite} onEdit={onEditAbsence} onCancel={setCancelling} />

      {covering !== null && coverBlock !== undefined && (
        <CoverDrawer board={board} index={index} block={coverBlock} date={covering.date} absentTeacherId={covering.class.teacherId}
          replaceOverrideId={covering.class.overrideId} onClose={() => setCovering(null)} onSaved={reread} />
      )}
      {cancelling !== null && (
        <ConfirmDialog title={`Cancelar la ausencia de ${index.teacherName(cancelling.teacherId)}`} confirmLabel="Cancelar ausencia"
          onConfirm={() => cancel(cancelling)} onCancel={() => setCancelling(null)}>
          Sus clases {absenceWhen(cancelling)} vuelven a su horario. La ausencia no se borra: queda en el historial como cancelada.
        </ConfirmDialog>
      )}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </div>
  );
}

function RangePicker({ range, onRange, today }: { range: UncoveredRange; onRange: (r: UncoveredRange) => void; today: string }): ReactElement {
  const thisWeek = schoolWeekOf(today);
  const chosen = range.kind === "today" ? "today" : range.monday === thisWeek ? "week" : "";
  return (
    <div style={{ display: "flex", gap: 12, alignItems: "center", flexWrap: "wrap" }}>
      <ChipRow label="Fechas" showCounts={false} chosen={chosen}
        options={[chipOption("today", "Hoy"), chipOption("week", "Esta semana")]}
        onChoose={(id) => onRange(id === "today" ? { kind: "today" } : { kind: "week", monday: thisWeek })} />
      {range.kind === "week" && <WeekStepper monday={range.monday} onMonday={(monday) => onRange({ kind: "week", monday })} />}
    </div>
  );
}

/** ‹ Semana del 5 al 9 de octubre › */
export function WeekStepper({ monday, onMonday }: { monday: string; onMonday: (monday: string) => void }): ReactElement {
  const step = { minHeight: 44, minWidth: 44, borderRadius: 99, border: "1.5px solid var(--border)", fontWeight: 700, fontSize: 18, color: "var(--text)" } as const;
  return (
    <div style={{ display: "inline-flex", alignItems: "center", gap: 10 }}>
      <button type="button" aria-label="Semana anterior" onClick={() => onMonday(addDays(monday, -WEEK))} style={step}>‹</button>
      <span style={{ fontWeight: 700, fontFamily: "var(--font-display)", color: "var(--text)" }}>{weekLabel(monday)}</span>
      <button type="button" aria-label="Semana siguiente" onClick={() => onMonday(addDays(monday, WEEK))} style={step}>›</button>
    </div>
  );
}

function RegisteredAbsences({ absences, index, canWrite, onEdit, onCancel }: {
  absences: readonly TeacherAbsence[];
  index: ScheduleIndex;
  canWrite: boolean;
  onEdit: (a: TeacherAbsence) => void;
  onCancel: (a: TeacherAbsence) => void;
}): ReactElement | null {
  if (absences.length === 0) return null;
  return (
    <section style={{ ...card, padding: "8px 0" }}>
      <h2 style={{ fontSize: 16, fontWeight: 700, color: "var(--text)", padding: "10px 20px" }}>Ausencias registradas</h2>
      <ul aria-label="Ausencias registradas" style={{ listStyle: "none", margin: 0, padding: 0 }}>
        {absences.map((a) => {
          const name = index.teacherName(a.teacherId);
          return (
            <li key={a.id} style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", padding: "10px 20px", borderTop: "1px solid var(--border)" }}>
              <span style={{ flex: "1 1 260px", minWidth: 0 }}>
                <strong>{name}</strong> — {absenceWhen(a)}
                {a.kind !== undefined && <span style={{ color: "var(--text-muted)" }}> · {ABSENCE_KIND_LABEL[a.kind]}</span>}
                {a.note !== undefined && a.note !== "" && <span style={{ display: "block", color: "var(--text-muted)", fontSize: 13.5 }}>Nota: {a.note}</span>}
                {a.createdByName !== undefined && <span style={{ display: "block", color: "var(--text-faint)", fontSize: 13 }}>Registró: {a.createdByName}</span>}
              </span>
              {canWrite && (
                <span style={{ display: "flex", gap: 8 }}>
                  <Btn kind="ghost" onClick={() => onEdit(a)} style={{ height: 44, fontSize: 14 }} aria-label={`Editar la ausencia de ${name}`}>Editar</Btn>
                  <Btn kind="ghost" onClick={() => onCancel(a)} style={{ height: 44, fontSize: 14, color: "var(--bad-fg)" }} aria-label={`Cancelar la ausencia de ${name}`}>Cancelar</Btn>
                </span>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { COLOR } from "../../_design/colors";
import type { ScheduleClass } from "../../schedule/api";
import type { ScheduleIndex } from "../../schedule/catalog";
import { subjectColor } from "../../schedule/time";
import { card, StatusBadge } from "./parts";
import { absentLine } from "./UncoveredView";

/** "Ana Ruiz", "Suplente: Luis Pérez (en lugar de Ana Ruiz)" or "Ana Ruiz · Ausente". */
function teacherLine(c: ScheduleClass, index: ScheduleIndex): string {
  if (c.status === "uncovered") return absentLine(index, c.teacherId, c.absence);
  if (c.status === "substitute") return `Suplente: ${index.teacherName(c.teacherId)} (en lugar de ${index.teacherName(c.baseTeacherId)})`;
  return index.teacherName(c.teacherId);
}

/** A swapped class says where it came from. */
function timeLine(c: ScheduleClass): string {
  const now = `${c.startsAt}–${c.endsAt}`;
  return c.status === "swapped" ? `${now} (antes ${c.baseStartsAt}–${c.baseEndsAt})` : now;
}

export interface ClassCardProps {
  cls: ScheduleClass;
  index: ScheduleIndex;
  /** Writers: cover an uncovered class (T19). */
  onCover?: (cls: ScheduleClass) => void;
}

/** One class happening at the chosen instant. */
export function ClassCard({ cls, index, onCover }: ClassCardProps): ReactElement {
  const subject = index.subjectName(cls.subjectId);
  const uncovered = cls.status === "uncovered";
  const color = uncovered ? "var(--bad-fg)" : COLOR[subjectColor(subject)].v;
  const cancelled = cls.status === "cancelled";
  return (
    <article data-testid="now-card" style={{ ...card, padding: "16px 18px", borderLeft: `6px solid ${color}`, display: "grid", gap: 6, minWidth: 0, opacity: cancelled ? 0.75 : 1 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10, justifyContent: "space-between", flexWrap: "wrap" }}>
        <strong style={{ fontFamily: "var(--font-display)", fontSize: 20, color: "var(--text)" }}>{index.groupLabel(cls.groupId)}</strong>
        <StatusBadge status={cls.status} />
      </div>
      <span style={{ fontWeight: 700, color: "var(--text)", textDecoration: cancelled ? "line-through" : "none" }}>{subject}</span>
      <span style={{ color: "var(--text-muted)", fontSize: 14.5 }}>{teacherLine(cls, index)}</span>
      <span style={{ color: "var(--text-faint)", fontSize: 13.5, fontVariantNumeric: "tabular-nums" }}>{timeLine(cls)}</span>
      {cls.reason !== null && cls.reason !== "" && <span style={{ color: "var(--text-muted)", fontSize: 13 }}>Motivo: {cls.reason}</span>}
      {uncovered && onCover !== undefined && (
        <Btn kind="outline" onClick={() => onCover(cls)} style={{ height: 44, fontSize: 14, justifySelf: "start" }}
          aria-label={`Asignar suplente a ${subject}, ${index.groupLabel(cls.groupId)}, ${cls.startsAt}`}>
          Asignar suplente
        </Btn>
      )}
    </article>
  );
}

export interface DayListProps {
  classes: readonly ScheduleClass[];
  index: ScheduleIndex;
  /** Writers: put a changed class back on its base timetable for that date. */
  onUndo?: (overrideId: string) => void;
}

/** Every class of the date, its changes visible; each change undoable by a writer. */
export function DayList({ classes, index, onUndo }: DayListProps): ReactElement {
  return (
    <section aria-label="Todas las clases del día" style={{ ...card, padding: "8px 0" }}>
      <h2 style={{ fontSize: 16, fontWeight: 700, color: "var(--text)", padding: "10px 20px" }}>Todas las clases del día</h2>
      <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
        {classes.map((c) => (
          <li key={c.blockId} style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", padding: "10px 20px", borderTop: "1px solid var(--border)" }}>
            <span style={{ fontVariantNumeric: "tabular-nums", color: "var(--text-muted)", minWidth: 96 }}>{c.startsAt}–{c.endsAt}</span>
            <span style={{ fontWeight: 700, minWidth: 56 }}>{index.groupLabel(c.groupId)}</span>
            <span style={{ flex: "1 1 220px", minWidth: 0, textDecoration: c.status === "cancelled" ? "line-through" : "none" }}>
              {index.subjectName(c.subjectId)} · {teacherLine(c, index)}
            </span>
            <StatusBadge status={c.status} />
            {onUndo !== undefined && c.overrideId !== null && (
              <Btn kind="ghost" onClick={() => onUndo(c.overrideId as string)} style={{ height: 44, fontSize: 14 }}
                aria-label={`Deshacer el cambio de ${index.subjectName(c.subjectId)}, ${index.groupLabel(c.groupId)}, ${c.startsAt}`}>
                Deshacer cambio
              </Btn>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

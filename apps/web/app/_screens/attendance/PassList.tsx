"use client";

import type { ReactElement } from "react";
import { useTranslations } from "next-intl";
import type { AttendanceStatus, RosterRow } from "../../attendance/api";
import { StatusPicker, type StatusOption } from "./parts";

export interface PassListProps {
  rows: readonly RosterRow[];
  /** Resolve a student's display name by id (attendance rows carry no names). */
  studentName: (studentId: string) => string;
  onStatusChange: (enrollmentId: string, status: AttendanceStatus) => void;
}

/** Present first: the default sits leftmost, where the thumb already is. */
const ORDER: readonly AttendanceStatus[] = ["present", "absent", "late", "excused"];

/**
 * The day's pass list: one row per enrolled student, each already PRESENT, with a
 * segmented control to mark the exception. Purely presentational — the roster and
 * the save live in `usePassList`.
 */
export function PassList({ rows, studentName, onStatusChange }: PassListProps): ReactElement {
  const t = useTranslations("attendance");
  const options: readonly StatusOption[] = ORDER.map((status) => ({ status, label: t(`status.${status}`) }));

  return (
    <table aria-label={t("list.label")} style={{ width: "100%", borderCollapse: "collapse" }}>
      <thead>
        <tr>
          <th style={{ textAlign: "left", padding: "0 8px 8px", fontSize: 13, fontWeight: 600, color: "var(--text-muted)" }}>
            {t("col.student")}
          </th>
          <th style={{ width: 1 }} />
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => {
          const name = studentName(row.studentId);
          return (
            <tr key={row.enrollmentId} style={{ borderTop: "1px solid var(--border)" }}>
              <td style={{ padding: "10px 8px", fontSize: 15, color: "var(--text)", fontWeight: 600 }}>{name}</td>
              <td style={{ padding: "10px 8px", textAlign: "right" }}>
                <StatusPicker
                  label={t("status.label", { name })}
                  options={options}
                  value={row.status}
                  onChange={(status) => onStatusChange(row.enrollmentId, status)}
                />
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

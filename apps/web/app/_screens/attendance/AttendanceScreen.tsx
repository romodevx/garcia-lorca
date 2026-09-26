"use client";

import { type ReactElement } from "react";

import { useFlag } from "@garcia-lorca/ui/flags";

import { DirectorAttendance } from "./DirectorAttendance";
import { TeacherAttendance } from "./TeacherAttendance";

export interface AttendanceScreenProps {
  onBack: () => void;
  /** Pins the day (tests, deep links); forwarded to whichever surface renders. */
  today?: string;
  /**
   * The authenticated principal's READ permissions. Selects the surface:
   * `attendance:read` → the full director/admin screen; otherwise
   * `attendance_own:read` → the self-scoped teacher screen. When both are held
   * the director wins (precedence: attendance:read first). UX-only — the gateway
   * (403/404) is the security boundary.
   */
  permissions?: readonly string[];
}

/**
 * Role-adaptive entry point for the Asistencia module. It only routes; the two
 * surfaces are self-contained. A director (attendance:read) gets the school-wide
 * DirectorAttendance; a teacher (attendance_own:read, without attendance:read)
 * gets the self-scoped TeacherAttendance, which never touches the
 * students/config/admin-attendance clients. When permissions are absent (e.g.
 * still resolving) it falls back to the director surface — the module would not
 * be visible to a principal holding neither permission anyway (see
 * isModuleVisible's any-of rule).
 */
export function AttendanceScreen({ onBack, today, permissions }: AttendanceScreenProps): ReactElement {
  const perms = permissions ?? [];
  /** The credential scanner rides on the student card module (UX only; the gateway enforces it). */
  const canScanCards = useFlag("module_student_card");
  if (!perms.includes("attendance:read") && perms.includes("attendance_own:read")) {
    return <TeacherAttendance onBack={onBack} today={today} canScanCards={canScanCards} />;
  }
  return <DirectorAttendance onBack={onBack} today={today} canScanCards={canScanCards} />;
}

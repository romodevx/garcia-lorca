"use client";

import { type ReactElement } from "react";
import { DirectorCapture } from "./DirectorCapture";
import { TeacherCapture } from "./TeacherCapture";

export interface GradesScreenProps {
  onBack: () => void;
  /**
   * The authenticated principal's READ permissions. Selects the surface:
   * `grades:read` → the full director screen; otherwise `teaching:read` → the
   * self-scoped teacher screen. When both are held the director wins (precedence:
   * grades:read first). UX-only — the gateway (403/404) is the security boundary.
   */
  permissions?: readonly string[];
}

/**
 * Role-adaptive entry point for the Calificaciones module. It only routes; the
 * two surfaces are self-contained. A director (grades:read) gets the school-wide
 * DirectorCapture; a teacher (teaching:read, without grades:read) gets the
 * self-scoped TeacherCapture, which never touches the students/grades/config
 * clients. When permissions are absent (e.g. still resolving) it falls back to
 * the director surface — the module would not be visible to a principal holding
 * neither permission anyway (see isModuleVisible's any-of rule).
 */
export function GradesScreen({ onBack, permissions }: GradesScreenProps): ReactElement {
  const perms = permissions ?? [];
  if (!perms.includes("grades:read") && perms.includes("teaching:read")) {
    return <TeacherCapture onBack={onBack} />;
  }
  return <DirectorCapture onBack={onBack} permissions={perms} />;
}

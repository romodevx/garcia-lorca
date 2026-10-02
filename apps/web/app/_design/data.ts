/**
 * Seed/fixture data for the web demo (Spanish UI). Ported from the design
 * handoff (project/data.jsx), scoped to the web system (administración +
 * maestros). Parent-app fixtures from the prototype are intentionally omitted.
 *
 * NOTE: what remains is CHROME data (the module panel + the demo identity in the
 * top bar). Domain fixtures are gone: every built module reads its own data from
 * the gateway — the security boundary — through `app/<module>/api.ts`.
 */
import type { ColorKey } from "./colors";
import type { IconName } from "./Icon";
import type { FlagKey } from "@garcia-lorca/ui/flags";

export interface ModuleDef {
  id: string;
  label: string;
  icon: IconName;
  color: ColorKey;
  desc: string;
  badge: number | null;
  /** Flag that gates this module's surface (UX-only; backend is authoritative). */
  flag: FlagKey;
  /**
   * READ permission(s) the authenticated principal must hold to SEE this module
   * in the panel/palette. A single string requires exactly that permission; an
   * array is ANY-OF — holding any one of the listed perms reveals the module (so
   * a surface with role-adaptive sub-views, like Calificaciones for directors OR
   * teachers, stays visible to either audience). An entry that is itself an
   * array is ALL-OF: the counter needs to read what is owed AND to key money,
   * so neither permission alone reveals it. UX-only: the gateway (403/404)
   * is the real security boundary. Modules with no backend (e.g. scholarships)
   * carry a permission nobody is ever granted, so they naturally disappear — no
   * special-casing.
   */
  perm: string | readonly (string | readonly string[])[];
  /**
   * Deactivated from the product (the school doesn't offer this module). Hidden
   * from the panel entirely — this is distinct from a flag being OFF, which
   * shows the module as available-but-disabled ("Próximamente"). Nothing is
   * deleted: the service, proto, and flag all remain, so re-enabling is just
   * removing this marker. See project memory [[module-docs-deactivated]].
   */
  hidden?: boolean;
}

export const MODULES: readonly ModuleDef[] = [
  { id: "students", label: "Estudiantes", icon: "students", color: "forest", desc: "Alumnos y admisión", badge: null, flag: "module_students", perm: "students:read" },
  { id: "teachers", label: "Docentes", icon: "user", color: "purple", desc: "Plantilla y asignaturas", badge: null, flag: "module_teachers", perm: ["teachers:read", "teachers:write"] },
  // Two doors on the same money, because two different people walk through
  // them: the front desk taking a payment, and the director reading the month.
  // Sentence case, like every other label and description in this list — and
  // like Spanish orthography, which capitalises only the first word. It read
  // "Registro de Pagos" until the palette's own copy disagreed with it.
  { id: "payments", label: "Registro de pagos", icon: "card", color: "forest", desc: "Cobrar en ventanilla", badge: null, flag: "module_finance", perm: [["collections:read", "payments:write"], ["finance:read", "finance:write"]] },
  { id: "finance", label: "Finanzas", icon: "finance", color: "orange", desc: "Ingresos y adeudos", badge: null, flag: "module_finance", perm: "finance:read" },
  // T10: what goes out, the balance and the reports. Its own permission
  // (expenses:read, delegable) — finance:read stays owner-only.
  { id: "expenses", label: "Egresos", icon: "trend", color: "red", desc: "Gastos, balance y reportes", badge: null, flag: "module_expenses", perm: "expenses:read" },
  // QR capture is deferred; the module ships as the daily pass list.
  // Any-of: directors/admins pass lists via attendance:read; teachers use their
  // self-scoped attendance_own:read surface. Either permission reveals the tile.
  { id: "attendance", label: "Asistencia", icon: "qr", color: "lime", desc: "Pase de lista diario", badge: null, flag: "module_attendance", perm: ["attendance:read", "attendance_own:read"] },
  // Any-of: directors capture via grades:read; teachers use their self-scoped
  // teaching:read surface. Either permission reveals the tile.
  { id: "grades", label: "Calificaciones", icon: "grades", color: "yellow", desc: "Boletas y captura", badge: null, flag: "module_grades", perm: ["grades:read", "teaching:read"] },
  // T16: the weekly timetable and the day's changes. Any-of: the director reads
  // it all (schedule:read); a teacher reads their own week (teaching:read).
  { id: "schedule", label: "Horarios", icon: "clock", color: "forest", desc: "Horario semanal y suplencias", badge: null, flag: "module_schedule", perm: ["schedule:read", "teaching:read"] },
  // T17: extracurriculars and their own charge. Its money lives in finance.
  { id: "activities", label: "Actividades", icon: "star", color: "yellow", desc: "Extracurriculares y su cobro", badge: null, flag: "module_activities", perm: "activities:read" },
  { id: "announcements", label: "Avisos", icon: "announcements", color: "orange", desc: "Comunicados y agenda", badge: null, flag: "module_comms", perm: "comms:read" },
  { id: "scholarships", label: "Becas", icon: "scholarship", color: "purple", desc: "Becas y descuentos", badge: null, flag: "module_scholarships", perm: "scholarships:read" },
  // Deactivated from the panel per the school (kept future-ready; nothing deleted). See [[module-docs-deactivated]].
  { id: "documents", label: "Documentos", icon: "documents", color: "forest", desc: "Constancias, kardex", badge: null, flag: "module_docs", perm: "documents:read", hidden: true },
  // Two doors, because the read permission is split: the storeroom
  // (uniforms:stock) resolves no student data and is grantable, while the order
  // list (uniforms:orders) carries every pupil's name and is owner-only. Either
  // one is a reason to show the tile — a Caja/Almacén role that cannot see the
  // module cannot do its job.
  { id: "uniforms", label: "Inventario", icon: "uniforms", color: "purple", desc: "Uniformes, libros y artículos", badge: null, flag: "module_uniforms", perm: ["uniforms:stock", "uniforms:orders"] },
  { id: "billing", label: "Facturación", icon: "billing", color: "lime", desc: "CFDI 4.0", badge: null, flag: "module_billing", perm: "billing:read" },
  { id: "reports", label: "Reportes", icon: "reports", color: "yellow", desc: "Indicadores", badge: null, flag: "module_reports", perm: "reports:read" },
  { id: "settings", label: "Configuración", icon: "settings", color: "neutral", desc: "Centro y permisos", badge: null, flag: "module_config", perm: "config:read" },
  // Admin surface (module_iam): users, roles, and the teacher↔user link.
  { id: "admin", label: "Administración", icon: "shield", color: "forest", desc: "Usuarios y roles", badge: null, flag: "module_iam", perm: "users:read" }
];

/**
 * Whether the principal holds a module's READ permission under the any-of rule:
 * a single-string `perm` must be present; an array `perm` matches if the
 * principal holds ANY one of its entries. Shared by the dashboard grid and the
 * command palette so their visibility can never drift.
 */
export function moduleHasPerm(m: ModuleDef, permissions: readonly string[]): boolean {
  const required = typeof m.perm === "string" ? [m.perm] : m.perm;
  const holds = (p: string): boolean => permissions.includes(p);
  return required.some((p) => (typeof p === "string" ? holds(p) : p.every(holds)));
}

/**
 * The module visibility rule, in ONE place so the dashboard grid and the command
 * palette can never drift. A module is shown IFF it is not deactivated AND the
 * principal holds (any of) its READ permission(s). UX-only — the gateway remains
 * the security boundary (its 403/404 is authoritative). No-backend modules carry
 * a permission nobody is granted, so they disappear naturally with no
 * special-casing.
 */
export function isModuleVisible(m: ModuleDef, permissions: readonly string[]): boolean {
  return m.hidden !== true && moduleHasPerm(m, permissions);
}

export type Role = "admin" | "teacher";

export interface UserProfile {
  name: string;
  first: string;
  role: string;
  color: ColorKey;
}

export const USERS: Record<Role, UserProfile> = {
  admin: { name: "Dirección", first: "Director", role: "Centro Educativo García Lorca", color: "forest" },
  teacher: { name: "Docente", first: "Profesor", role: "Personal académico", color: "purple" }
};

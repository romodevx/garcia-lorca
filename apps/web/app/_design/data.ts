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
   * Deactivated from the product (the school doesn't offer this module). Hidden
   * from the panel entirely — this is distinct from a flag being OFF, which
   * shows the module as available-but-disabled ("Próximamente"). Nothing is
   * deleted: the service, proto, and flag all remain, so re-enabling is just
   * removing this marker. See project memory [[module-docs-deactivated]].
   */
  hidden?: boolean;
}

export const MODULES: readonly ModuleDef[] = [
  { id: "students", label: "Estudiantes", icon: "students", color: "forest", desc: "Alumnos y admisión", badge: null, flag: "module_students" },
  { id: "finance", label: "Finanzas y Adeudos", icon: "finance", color: "orange", desc: "Colegiaturas y adeudos", badge: null, flag: "module_finance" },
  // QR capture is deferred; the module ships as the daily pass list.
  { id: "attendance", label: "Asistencia", icon: "qr", color: "lime", desc: "Pase de lista diario", badge: null, flag: "module_attendance" },
  { id: "grades", label: "Calificaciones", icon: "grades", color: "yellow", desc: "Boletas y captura", badge: null, flag: "module_grades" },
  { id: "announcements", label: "Avisos", icon: "announcements", color: "orange", desc: "Comunicados y agenda", badge: null, flag: "module_comms" },
  { id: "scholarships", label: "Becas", icon: "scholarship", color: "purple", desc: "Becas y descuentos", badge: null, flag: "module_scholarships" },
  // Deactivated from the panel per the school (kept future-ready; nothing deleted). See [[module-docs-deactivated]].
  { id: "documents", label: "Documentos", icon: "documents", color: "forest", desc: "Constancias, kardex", badge: null, flag: "module_docs", hidden: true },
  { id: "uniforms", label: "Uniformes", icon: "uniforms", color: "purple", desc: "Pedidos y tallas", badge: null, flag: "module_uniforms" },
  { id: "billing", label: "Facturación", icon: "billing", color: "lime", desc: "CFDI 4.0", badge: null, flag: "module_billing" },
  { id: "reports", label: "Reportes", icon: "reports", color: "yellow", desc: "Indicadores", badge: null, flag: "module_reports" },
  { id: "settings", label: "Configuración", icon: "settings", color: "neutral", desc: "Centro y permisos", badge: null, flag: "module_config" },
  // Admin surface (module_iam): users, roles, and the teacher↔user link.
  { id: "admin", label: "Administración", icon: "shield", color: "forest", desc: "Usuarios y roles", badge: null, flag: "module_iam" }
];

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

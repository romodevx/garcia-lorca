/**
 * Seed/fixture data for the web demo (Spanish UI). Ported from the design
 * handoff (project/data.jsx), scoped to the web system (administración +
 * maestros). Parent-app fixtures from the prototype are intentionally omitted.
 *
 * NOTE: this is mock data for the presentation surface. When `module_finance`
 * et al. land their real backends, these screens read from
 * `@garcia-lorca/api-client` instead — the gateway is the security boundary.
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
}

export const MODULES: readonly ModuleDef[] = [
  { id: "students", label: "Estudiantes", icon: "students", color: "forest", desc: "1,248 alumnos", badge: null, flag: "module_students" },
  { id: "finance", label: "Finanzas y Adeudos", icon: "finance", color: "orange", desc: "37 con adeudo", badge: 37, flag: "module_finance" },
  { id: "attendance", label: "Asistencia QR", icon: "qr", color: "lime", desc: "92% hoy", badge: null, flag: "module_attendance" },
  { id: "grades", label: "Calificaciones", icon: "grades", color: "yellow", desc: "Bloque 3 abierto", badge: null, flag: "module_grades" },
  { id: "announcements", label: "Avisos", icon: "announcements", color: "orange", desc: "2 nuevos", badge: 2, flag: "module_comms" },
  { id: "scholarships", label: "Becas", icon: "scholarship", color: "purple", desc: "84 activas", badge: null, flag: "module_scholarships" },
  { id: "documents", label: "Documentos", icon: "documents", color: "forest", desc: "Constancias, kardex", badge: null, flag: "module_docs" },
  { id: "uniforms", label: "Uniformes", icon: "uniforms", color: "purple", desc: "Pedidos y tallas", badge: null, flag: "module_uniforms" },
  { id: "billing", label: "Facturación", icon: "billing", color: "lime", desc: "CFDI 4.0", badge: null, flag: "module_billing" },
  { id: "reports", label: "Reportes", icon: "reports", color: "yellow", desc: "Indicadores", badge: null, flag: "module_reports" },
  { id: "settings", label: "Configuración", icon: "settings", color: "neutral", desc: "Centro y permisos", badge: null, flag: "module_config" }
];

export const LEVELS = ["Kínder", "Primaria", "Secundaria", "Preparatoria"] as const;
export type Level = (typeof LEVELS)[number];

export type StudentStatus = "paid" | "due" | "overdue";

export interface StudentRow {
  id: string;
  name: string;
  level: Level;
  grade: string;
  guardian: string;
  amount: number;
  status: StudentStatus;
  due: string;
  concept: string;
}

export const STUDENTS: readonly StudentRow[] = [
  { id: "GL-2041", name: "Sofía Hernández Ríos", level: "Primaria", grade: "4° A", guardian: "Marisol Ríos", amount: 0, status: "paid", due: "—", concept: "Colegiatura junio" },
  { id: "GL-1187", name: "Mateo Domínguez Lara", level: "Secundaria", grade: "2° B", guardian: "Jorge Domínguez", amount: 3200, status: "overdue", due: "5 jun", concept: "Colegiatura mayo" },
  { id: "GL-3320", name: "Valentina Cruz Mendoza", level: "Preparatoria", grade: "1° C", guardian: "Ana Mendoza", amount: 1850, status: "due", due: "15 jun", concept: "Colegiatura junio" },
  { id: "GL-2298", name: "Diego Navarro Estrada", level: "Primaria", grade: "6° A", guardian: "Luis Navarro", amount: 0, status: "paid", due: "—", concept: "Colegiatura junio" },
  { id: "GL-0455", name: "Renata Flores Aguilar", level: "Kínder", grade: "K3", guardian: "Paola Aguilar", amount: 5400, status: "overdue", due: "28 may", concept: "Colegiatura + inscripción" },
  { id: "GL-2710", name: "Emiliano Reyes Castro", level: "Secundaria", grade: "1° A", guardian: "Sandra Castro", amount: 0, status: "paid", due: "—", concept: "Colegiatura junio" },
  { id: "GL-1903", name: "Camila Ortega Salinas", level: "Preparatoria", grade: "2° A", guardian: "Hugo Ortega", amount: 1850, status: "due", due: "15 jun", concept: "Colegiatura junio" },
  { id: "GL-3088", name: "Santiago Vega Morales", level: "Primaria", grade: "3° B", guardian: "Karla Morales", amount: 0, status: "paid", due: "—", concept: "Colegiatura junio" },
  { id: "GL-2566", name: "Isabella Ramírez Peña", level: "Kínder", grade: "K2", guardian: "Diana Peña", amount: 2700, status: "overdue", due: "1 jun", concept: "Colegiatura mayo" },
  { id: "GL-1422", name: "Sebastián Luna Carrillo", level: "Secundaria", grade: "3° C", guardian: "Raúl Luna", amount: 0, status: "paid", due: "—", concept: "Colegiatura junio" },
  { id: "GL-2845", name: "Regina Campos Téllez", level: "Primaria", grade: "5° A", guardian: "Mónica Téllez", amount: 1600, status: "due", due: "18 jun", concept: "Material y laboratorio" },
  { id: "GL-3501", name: "Leonardo Fuentes Bravo", level: "Preparatoria", grade: "3° B", guardian: "Pedro Fuentes", amount: 0, status: "paid", due: "—", concept: "Colegiatura junio" }
];

export interface FinanceSummary {
  collected: number;
  pending: number;
  overdueCount: number;
  paidPct: number;
}

export const FIN_SUMMARY: FinanceSummary = {
  collected: 842500,
  pending: 128300,
  overdueCount: 37,
  paidPct: 87
};

export type Role = "admin" | "teacher";

export interface UserProfile {
  name: string;
  first: string;
  role: string;
  color: ColorKey;
}

export const USERS: Record<Role, UserProfile> = {
  admin: { name: "Laura Mendizábal", first: "Laura", role: "Dirección", color: "forest" },
  teacher: { name: "Miguel Vázquez", first: "Miguel", role: "Docente · 4° A", color: "purple" }
};

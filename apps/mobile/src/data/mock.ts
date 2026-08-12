/**
 * Mock data for the García Lorca parent app (single child), es-MX. Ported from
 * the design handoff (data.jsx) to typed TypeScript. This stands in for the
 * gateway read-models until the real API client (generated from the OpenAPI
 * contract) is wired in.
 */
import type { IconName } from "../icons";
import type { TintName } from "../theme";

export type AttendanceStatus =
  | "present"
  | "absent"
  | "excused"
  | "retardo"
  | "holiday"
  | "future";

export type GradingStyle = "num" | "qual";
export type LevelId = "kinder" | "primaria" | "secundaria";
export type QualLevel = "logrado" | "proceso" | "apoyo";
export type ChipKind = "ok" | "warn" | "bad" | "neutral" | "lime" | "purple";

export interface Child {
  name: string;
  short: string;
  initials: string;
  parent: string;
  teacher: string;
  avatar: { from: string; to: string };
}

export interface Level {
  id: LevelId;
  name: string;
  group: string;
  grading: GradingStyle;
}

export interface NumericSubject {
  name: string;
  grade: number;
  icon: IconName;
  tint: TintName;
  terms: number[];
}

export interface QualSubject {
  name: string;
  level: QualLevel;
  icon: IconName;
}

export interface Movement {
  date: string;
  title: string;
  sub: string;
  amount: number;
  kind: "pago" | "cargo" | "descuento";
  paid?: boolean;
}

export interface SchoolEvent {
  day: string;
  mon: string;
  title: string;
  sub: string;
  tint: TintName;
}

export interface ModuleTile {
  id: "documentos" | "uniformes" | "facturas" | "becas";
  title: string;
  sub: string;
  icon: IconName;
  tint: TintName;
  /**
   * Deactivated from the product (the school doesn't offer this module). Hidden
   * from the "Más" grid entirely — distinct from a flag being OFF, which shows
   * the tile as "Próximamente". Nothing is deleted; re-enabling is just removing
   * this marker. See project memory [[module-docs-deactivated]].
   */
  hidden?: boolean;
}

export interface ExcuseReason {
  id: string;
  label: string;
}

// TODO(pendiente-backend: perfil del hijo family — students+config exist, falta ruta family)
// — dato estático provisional. No borrar. Ver .claude/docs/PLAN-mock-data-audit.md #B5.
export const child: Child = {
  name: "Mateo Herrera Díaz",
  short: "Mateo",
  initials: "MH",
  parent: "Laura",
  teacher: "Mtra. Patricia Lugo",
  avatar: { from: "#7AC400", to: "#0A7A2F" },
};

export const levels: Record<LevelId, Level> = {
  kinder: { id: "kinder", name: "Kínder", group: "K3 · Girasoles", grading: "qual" },
  primaria: { id: "primaria", name: "Primaria", group: "3° A", grading: "num" },
  secundaria: { id: "secundaria", name: "Secundaria", group: "2° B", grading: "num" },
};

// The home "Hoy" summary. `unread` was removed: the avisos counter is now derived
// from real /family/notifications data (AppState.unreadCount). The remaining
// fields are still static.
// TODO(pendiente-backend: GET /api/v1/family/students/{id}/attendance | .../account | agenda)
// — dato estático provisional. No borrar. Ver .claude/docs/PLAN-mock-data-audit.md #B3/#B4/#B6.
export const today = {
  dateLabel: "vie 12 jun",
  attendance: { status: "present" as AttendanceStatus, label: "Presente", detail: "Entrada 7:52 a.m." },
  balance: { amount: 1850, status: "warn", label: "Pendiente", due: "Vence 15 jun" },
  nextEvent: { title: "Festival de Primavera", when: "vie 20 jun · 9:00 a.m.", cat: "evento" },
};

// TODO(pendiente-backend: GET /api/v1/family/students/{id}/attendance)
// — dato estático provisional. No borrar. Ver .claude/docs/PLAN-mock-data-audit.md #B3.
export const attendance = {
  monthLabel: "Junio 2026",
  todayDate: 12,
  firstDow: 1, // Monday=1 (Sunday=0)
  days: {
    1: "present", 2: "present", 3: "present", 4: "absent", 5: "present",
    8: "present", 9: "present", 10: "excused", 11: "present", 12: "present",
    15: "future", 16: "holiday", 17: "future", 18: "future", 19: "future",
    22: "future", 23: "future", 24: "future", 25: "future", 26: "future",
    29: "future", 30: "future",
  } as Record<number, AttendanceStatus>,
  stats: { pct: 96, faltas: 1, justificadas: 1, retardos: 2 },
  log: [
    { date: "jue 4 jun", day: 4, status: "absent" as AttendanceStatus, note: "Falta sin justificar" },
    { date: "mié 10 jun", day: 10, status: "excused" as AttendanceStatus, note: "Justificada · Cita médica" },
    { date: "lun 2 jun", day: 2, status: "retardo" as AttendanceStatus, note: "Retardo · 8:10 a.m." },
  ],
};

// TODO(pendiente-backend: GET /api/v1/family/students/{id}/grades — reusa grades service)
// — dato estático provisional. No borrar. Ver .claude/docs/PLAN-mock-data-audit.md #B2.
export const gradesNum = {
  average: 9.3,
  period: "2.º periodo",
  subjects: [
    { name: "Español", grade: 9.2, icon: "book", tint: "forest", terms: [9.0, 9.1, 9.5] },
    { name: "Matemáticas", grade: 8.5, icon: "chart", tint: "orange", terms: [8.0, 8.6, 8.9] },
    { name: "Ciencias Naturales", grade: 9.6, icon: "leaf", tint: "lime", terms: [9.4, 9.6, 9.8] },
    { name: "Historia", grade: 8.8, icon: "clock", tint: "purple", terms: [8.5, 8.8, 9.1] },
    { name: "Geografía", grade: 9.0, icon: "map", tint: "forest", terms: [8.8, 9.0, 9.2] },
    { name: "Formación C. y Ética", grade: 9.8, icon: "heart", tint: "purple", terms: [9.7, 9.8, 9.9] },
    { name: "Inglés", grade: 9.4, icon: "star", tint: "lime", terms: [9.2, 9.4, 9.6] },
    { name: "Educación Física", grade: 10, icon: "flame", tint: "orange", terms: [10, 10, 10] },
  ] satisfies NumericSubject[],
};

export const gradesQual = {
  period: "2.º periodo",
  summary: { logrado: 4, proceso: 2, apoyo: 0 },
  subjects: [
    { name: "Lenguaje y Comunicación", level: "logrado", icon: "book" },
    { name: "Pensamiento Matemático", level: "proceso", icon: "chart" },
    { name: "Exploración del Mundo", level: "logrado", icon: "leaf" },
    { name: "Desarrollo Personal y Social", level: "logrado", icon: "heart" },
    { name: "Artes", level: "logrado", icon: "sparkles" },
    { name: "Educación Física", level: "proceso", icon: "flame" },
  ] satisfies QualSubject[],
};

export const qualMeta: Record<QualLevel, { label: string; chip: ChipKind; pct: number }> = {
  logrado: { label: "Logrado", chip: "ok", pct: 100 },
  proceso: { label: "En proceso", chip: "warn", pct: 60 },
  apoyo: { label: "Requiere apoyo", chip: "bad", pct: 30 },
};

// TODO(pendiente-backend: GET /api/v1/family/students/{id}/account — reusa finance.GetAccount)
// — dato estático provisional. No borrar. Ver .claude/docs/PLAN-mock-data-audit.md #B4.
export const balance = {
  amount: 1850,
  status: "warn",
  label: "Pendiente",
  due: "Vence 15 de junio",
  movements: [
    { date: "02 jun 2026", title: "Pago recibido", sub: "Colegiatura mayo", amount: -2400, kind: "pago" },
    { date: "01 jun 2026", title: "Colegiatura junio", sub: "Cargo mensual", amount: 2400, kind: "cargo" },
    { date: "28 may 2026", title: "Beca aplicada", sub: "Descuento 23%", amount: -550, kind: "descuento" },
    { date: "15 may 2026", title: "Material didáctico", sub: "Pagado", amount: 300, kind: "cargo", paid: true },
    { date: "02 may 2026", title: "Pago recibido", sub: "Colegiatura abril", amount: -2400, kind: "pago" },
  ] satisfies Movement[],
};

// NOTE: the former `announcements` / `catMeta` mock was removed — avisos now load
// from the real gateway family surface (GET /api/v1/family/notifications) via the
// API client (src/api/notifications.ts) and are rendered through
// src/screens/notificationView.ts. See .claude/docs/PLAN-mock-data-audit.md #A4/#A5.

// TODO(pendiente-backend: agenda/eventos — no endpoint; comms agenda diferida Phase 9d)
// — dato estático provisional. No borrar. Ver .claude/docs/PLAN-mock-data-audit.md #B6.
export const events: SchoolEvent[] = [
  { day: "16", mon: "JUN", title: "Suspensión de clases", sub: "Consejo Técnico Escolar", tint: "orange" },
  { day: "18", mon: "JUN", title: "Reunión de padres", sub: "5:00 p.m. · Salón 3°A", tint: "forest" },
  { day: "20", mon: "JUN", title: "Festival de Primavera", sub: "9:00 a.m. · Patio central", tint: "purple" },
];

export const modules: ModuleTile[] = [
  // Deactivated from the panel per the school (kept future-ready; nothing deleted). See [[module-docs-deactivated]].
  { id: "documentos", title: "Documentos", sub: "Boletas y constancias", icon: "document", tint: "forest", hidden: true },
  { id: "uniformes", title: "Uniformes", sub: "Tienda y tallas", icon: "shirt", tint: "lime" },
  { id: "facturas", title: "Facturas", sub: "CFDI y recibos", icon: "receipt", tint: "orange" },
  { id: "becas", title: "Becas", sub: "Estado de tu beca", icon: "cap", tint: "purple" },
];

export const reasons: ExcuseReason[] = [
  { id: "medica", label: "Cita médica" },
  { id: "familiar", label: "Asunto familiar" },
  { id: "enfermedad", label: "Enfermedad" },
  { id: "otro", label: "Otro motivo" },
];

/** Formats a peso amount (es-MX), prefixing "-$" for negatives. */
export function money(n: number): string {
  const abs = Math.abs(n).toLocaleString("es-MX", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return (n < 0 ? "-$" : "$") + abs;
}

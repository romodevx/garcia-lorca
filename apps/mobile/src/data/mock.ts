/**
 * Remaining static content for the García Lorca parent app, es-MX.
 *
 * The child context, account, attendance, grades and avisos surfaces are now
 * backed by the real gateway `/family/*` read models (see src/api/family.ts,
 * src/state/AppState.tsx and the wired screens). What stays here is the content
 * that has NO backend yet and is kept future-ready (agenda events, the "Más"
 * module tiles, excuse reasons) plus the shared `money` formatter and the chip
 * kinds. Each unbacked block keeps its TODO marker until its endpoint lands.
 */
import type { IconName } from "../icons";
import type { TintName } from "../theme";

export type ChipKind = "ok" | "warn" | "late" | "bad" | "neutral" | "lime" | "purple";

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

// TODO(pendiente-backend: agenda/events — no endpoint; comms agenda deferred to Phase 9d)
// — static placeholder data. Do not delete. See .claude/docs/PLAN-mock-data-audit.md #B6.
export const events: SchoolEvent[] = [
  { day: "16", mon: "JUN", title: "Suspensión de clases", sub: "Consejo Técnico Escolar", tint: "orange" },
  { day: "18", mon: "JUN", title: "Reunión de padres", sub: "5:00 p.m. · Salón 3°A", tint: "forest" },
  { day: "20", mon: "JUN", title: "Festival de Primavera", sub: "9:00 a.m. · Patio central", tint: "purple" },
];

export const modules: ModuleTile[] = [
  // Deactivated from the panel per the school (kept future-ready; nothing deleted). See [[module-docs-deactivated]].
  { id: "documentos", title: "Documentos", sub: "Constancias y reglamento", icon: "document", tint: "forest", hidden: true },
  { id: "uniformes", title: "Uniformes", sub: "Tienda y tallas", icon: "shirt", tint: "lime" },
  { id: "facturas", title: "Facturas", sub: "CFDI y recibos", icon: "receipt", tint: "orange" },
  // Hidden: since T18 a family's beca shows where it matters — as a "Beca" line
  // on each charge of the account statement. This tile's content is a mock
  // (no family endpoint for the beca itself), so it stays unreachable.
  { id: "becas", title: "Becas", sub: "Estado de tu beca", icon: "cap", tint: "purple", hidden: true },
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

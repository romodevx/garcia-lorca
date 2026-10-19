import type { FlagSet } from "@garcia-lorca/ui/flags";

import type { ColorKey } from "../_design/colors";
import type { IconName } from "../_design/Icon";
import { MODULES, isModuleVisible } from "../_design/data";
import { formatCents as money } from "../_lib/money";
import { homeShortcuts } from "./shortcuts";
import type { HomeKpis, Ledger } from "./useHomeKpis";

/** Where a click goes: a module, optionally on one of its intents. */
export interface KpiTarget {
  id: string;
  intent?: string;
}

/**
 * What a KPI card shows (PLAN-redesign-home §3.4). Copy is named by its key in
 * the `home` namespace; the card translates it.
 */
export type KpiFace =
  | { kind: "loading" }
  | { kind: "error"; retry: () => void }
  | { kind: "noCycle"; toConfig: boolean }
  | { kind: "zero"; textKey: string; ctaKey: string; target: KpiTarget }
  | { kind: "value"; value: string; subKey: string; subValues?: Record<string, string | number> };

export interface KpiCard {
  id: "students" | "pending" | "collected";
  labelKey: string;
  icon: IconName;
  color: ColorKey;
  /** Where the card goes — unless it shows a zero, which goes to its next step. */
  opens: KpiTarget;
  face: KpiFace;
}

const LOADING: KpiFace = { kind: "loading" };
const payments = MODULES.find((m) => m.id === "payments");
const settings = MODULES.find((m) => m.id === "settings");

/**
 * Whether the caller can actually open Configuración (§3.4): its card is
 * visible AND its flag is on — with the flag off it lands on a placeholder.
 */
function opensSettings(permissions: readonly string[], flags: FlagSet): boolean {
  return settings !== undefined && isModuleVisible(settings, permissions) && flags[settings.flag] === true;
}

function studentsFace(k: HomeKpis, permissions: readonly string[], flags: FlagSet): KpiFace {
  const s = k.students;
  if (s.status === "error") return { kind: "error", retry: k.retryStudents };
  if (s.status !== "ready") return LOADING;
  if (s.value > 0) return { kind: "value", value: String(s.value), subKey: "kpi.students.sub" };
  const canCreate = homeShortcuts("students", permissions, flags).some((sc) => sc.intent === "new");
  return canCreate
    ? { kind: "zero", textKey: "kpi.students.zero", ctaKey: "kpi.students.new", target: { id: "students", intent: "new" } }
    : { kind: "zero", textKey: "kpi.students.zero", ctaKey: "kpi.students.go", target: { id: "students" } };
}

/** Shared by both money KPIs: the states the ledger read and the cycle decide. */
function ledgerFace(k: HomeKpis, toConfig: boolean, ready: (l: Ledger) => KpiFace): KpiFace {
  const l = k.ledger;
  if (l.status === "error") return { kind: "error", retry: k.retryLedger };
  if (l.status === "noCycle" || k.cycle?.active === false) return { kind: "noCycle", toConfig };
  if (l.status !== "ready") return LOADING;
  return ready(l.value);
}

/**
 * The KPI cards this caller sees, and the face each one shows. Students for
 * students:read (B4); the money pair for finance:read (B5) plus config:read,
 * since the ledger is read per cycle and only config:read reads cycles, and
 * only while module_finance is on (C2-013).
 */
export function homeKpiCards(k: HomeKpis, permissions: readonly string[], flags: FlagSet): KpiCard[] {
  const can = (p: string): boolean => permissions.includes(p);
  const cards: KpiCard[] = [];
  if (can("students:read")) {
    cards.push({ id: "students", labelKey: "kpi.students.label", icon: "students", color: "forest", opens: { id: "students" }, face: studentsFace(k, permissions, flags) });
  }
  if (!can("finance:read") || !can("config:read") || flags.module_finance !== true) return cards;

  const toConfig = opensSettings(permissions, flags);
  const toPayments = payments !== undefined && isModuleVisible(payments, permissions);
  cards.push({
    id: "pending", labelKey: "kpi.pending.label", icon: "finance", color: "orange", opens: { id: "finance" },
    face: ledgerFace(k, toConfig, (l) =>
      l.charges === 0
        ? { kind: "zero", textKey: "kpi.pending.zero", ctaKey: "kpi.pending.go", target: { id: "finance" } }
        : { kind: "value", value: money(l.pendingCents), subKey: "kpi.pending.sub", subValues: { count: l.withBalance } }),
  });
  cards.push({
    id: "collected", labelKey: "kpi.collected.label", icon: "trend", color: "lime", opens: { id: "finance" },
    face: ledgerFace(k, toConfig, (l) =>
      l.collectedCents > 0
        ? { kind: "value", value: money(l.collectedCents), subKey: "kpi.collected.sub", subValues: { cycle: k.cycle?.name ?? "—" } }
        : toPayments
          ? { kind: "zero", textKey: "kpi.collected.zero", ctaKey: "kpi.collected.toPayments", target: { id: "payments" } }
          : { kind: "zero", textKey: "kpi.collected.zero", ctaKey: "kpi.collected.toFinance", target: { id: "finance" } }),
  });
  return cards;
}

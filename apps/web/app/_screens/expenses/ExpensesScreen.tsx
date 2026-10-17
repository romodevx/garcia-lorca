"use client";

import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Toast } from "../../_design/Toast";
import { API_URL } from "../../api-url";
import { getSummary, listCategories, listExpenses, type Expense, type ExpenseCategory, type FinanceSummary } from "../../expenses/api";
import { formatCents } from "../../_lib/money";
import { dayWithYearLabel, localISODate } from "../../_lib/localDate";
import { StatCards } from "../uniforms/parts";
import { CategoriesPanel } from "./CategoriesPanel";
import { useIntent } from "../../routing/pendingIntent";
import { ExpenseDrawer, METHOD_LABEL } from "./ExpenseDrawer";
import { monthLabel, monthRange, shiftMonth } from "./period";
import { ReportsPanel } from "./ReportsPanel";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";

type TabId = "expenses" | "reports" | "categories";
type StatusFilter = "active" | "cancelled";

const PAGE_SIZE = 20;
const TOAST_MS = 3000;
const FAB_HEIGHT = 60;
const FAB_BOTTOM = 32;
const FAB_GAP = 16;

export interface ExpensesScreenProps {
  onBack: () => void;
  permissions?: readonly string[];
  /** The office's day (aaaa-mm-dd); injected by tests. */
  today?: string;
}

/**
 * Egresos (T10): what the school pays out, the month's balance and the
 * reports. Header, stat cards, tabs, the table, a quick-add drawer and the FAB
 * (DESIGN-SYSTEM.md). Reading needs expenses:read; recording, editing,
 * cancelling and the categories need expenses:write — the gateway decides.
 */
export function ExpensesScreen({ onBack, permissions, today: injectedToday }: ExpensesScreenProps): ReactElement {
  const today = injectedToday ?? localISODate();
  const canWrite = (permissions ?? []).includes("expenses:write");
  const [tab, setTab] = useState<TabId>("expenses");
  const [month, setMonth] = useState(today);
  const [status, setStatus] = useState<StatusFilter>("active");
  const [page, setPage] = useState(0);
  // null until read: "no categories" is only said once it is known.
  const [categories, setCategories] = useState<ExpenseCategory[] | null>(null);
  const [focusNewCategory, setFocusNewCategory] = useState(false);
  const [expenses, setExpenses] = useState<Expense[] | null>(null);
  const [summary, setSummary] = useState<FinanceSummary | null>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const intent = useIntent("expenses");
  const [drawer, setDrawer] = useState<{ expense?: Expense } | null>(() => (intent === "new" && canWrite ? {} : null));
  const [toast, setToast] = useState<string | undefined>(undefined);
  const [reads, setReads] = useState(0);
  const range = useMemo(() => monthRange(month), [month]);

  const loadCategories = useCallback(() => {
    listCategories(API_URL, true).then(setCategories).catch(() => setCategories([]));
  }, []);
  useEffect(loadCategories, [loadCategories]);

  useEffect(() => {
    let live = true;
    setFailure(undefined);
    Promise.all([
      listExpenses(API_URL, { from: range.from, to: range.to, includeCancelled: true }),
      getSummary(API_URL, range.from, range.to),
    ])
      .then(([list, sum]) => {
        if (!live) return;
        setExpenses(list);
        setSummary(sum);
      })
      .catch((err: unknown) => {
        if (!live) return;
        setExpenses([]);
        setFailure(err instanceof Error ? err.message : "No se pudieron leer los egresos.");
      });
    return () => {
      live = false;
    };
  }, [range, reads]);

  useEffect(() => {
    if (toast === undefined) return;
    const t = setTimeout(() => setToast(undefined), TOAST_MS);
    return () => clearTimeout(t);
  }, [toast]);

  const shown = (expenses ?? []).filter((e) => e.status === status);
  const pages = Math.max(1, Math.ceil(shown.length / PAGE_SIZE));
  const visible = shown.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const cancelledCount = (expenses ?? []).filter((e) => e.status === "cancelled").length;

  const cards = [
    { icon: "finance" as const, color: "forest" as const, label: "Ingresos", value: formatCents(summary?.income.totalCents ?? 0), sub: "dinero recibido en el mes", testId: "stat-income" },
    { icon: "trend" as const, color: "red" as const, label: "Egresos", value: formatCents(summary?.expenses.totalCents ?? 0), sub: `${shown.length} ${status === "active" ? "registrados" : "cancelados"}`, testId: "stat-expenses" },
    { icon: "card" as const, color: "purple" as const, label: "Balance", value: formatCents(summary?.balance.totalCents ?? 0), sub: "ingresos − egresos", testId: "stat-balance" },
    { icon: "documents" as const, color: "orange" as const, label: "Notas de crédito", value: formatCents(summary?.creditNoteCents ?? 0), sub: "no es ingreso", testId: "stat-credit-notes" },
  ];

  const noActiveCategory = categories !== null && !categories.some((c) => c.active);
  const clearFocusNewCategory = useCallback(() => setFocusNewCategory(false), []);
  const createFirstCategory = (): void => {
    setDrawer(null);
    setTab("categories");
    setFocusNewCategory(true);
  };

  const onSaved = (message: string, keepOpen?: boolean): void => {
    setToast(message);
    setReads((n) => n + 1);
    if (keepOpen !== true) setDrawer(null);
  };

  const tabs: { id: TabId; label: string }[] = [
    { id: "expenses", label: "Egresos" },
    { id: "reports", label: "Balance y reportes" },
    { id: "categories", label: "Categorías" },
  ];

  return (
    <Page>
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <header style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24, flexWrap: "wrap" }}>
        <ModuleHeaderIcon icon="trend" color="forest" />
        <div style={{ minWidth: 0, flex: 1 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Egresos</h1>
          <div style={{ display: "flex", alignItems: "center", gap: 6, marginTop: 3 }}>
            <button type="button" aria-label="Mes anterior" onClick={() => { setMonth(shiftMonth(month, -1)); setPage(0); }} style={{ minWidth: 44, minHeight: 44, display: "grid", placeItems: "center", color: "var(--text-muted)" }}>
              <Icon name="back" size={18} />
            </button>
            <p style={{ color: "var(--text-muted)", fontSize: 15 }}>{monthLabel(month)}</p>
            <button type="button" aria-label="Mes siguiente" disabled={range.to >= today} onClick={() => { setMonth(shiftMonth(month, 1)); setPage(0); }} style={{ minWidth: 44, minHeight: 44, display: "grid", placeItems: "center", color: "var(--text-muted)", opacity: range.to >= today ? 0.35 : 1 }}>
              <Icon name="chevron" size={18} />
            </button>
          </div>
        </div>
        {canWrite && <Btn icon="plus" onClick={() => setDrawer({})}>Registrar egreso</Btn>}
      </header>

      <div style={{ display: "grid", gap: 18 }}>
        {failure !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14.5 }}>{failure}</p>}

        <div>
          <StatCards cards={cards} />
        </div>

        <div role="tablist" aria-label="Secciones de egresos" style={{ display: "flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", flexWrap: "wrap", alignSelf: "start" }}>
          {tabs.map((t) => {
            const on = t.id === tab;
            return (
              <button key={t.id} role="tab" aria-selected={on} onClick={() => setTab(t.id)}
                style={{ height: 42, padding: "0 18px", borderRadius: 99, background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 600, fontSize: 14.5, fontFamily: "var(--font-display)", boxShadow: on ? "var(--shadow-card)" : "none" }}>
                {t.label}
              </button>
            );
          })}
        </div>

        {tab === "expenses" && (
          <ExpensesTable
            rows={visible}
            loading={expenses === null}
            status={status}
            cancelledCount={cancelledCount}
            activeCount={(expenses ?? []).length - cancelledCount}
            onStatus={(s) => { setStatus(s); setPage(0); }}
            onOpen={(e) => setDrawer({ expense: e })}
            page={page}
            pages={pages}
            total={shown.length}
            onPage={setPage}
          />
        )}
        {tab === "reports" && <ReportsPanel today={today} />}
        {tab === "categories" && <CategoriesPanel categories={categories ?? []} canWrite={canWrite} onChanged={loadCategories} focusNew={focusNewCategory} onFocused={clearFocusNewCategory} />}
      </div>

      {canWrite && (
        <>
          <div aria-hidden style={{ height: FAB_BOTTOM + FAB_HEIGHT + FAB_GAP }} />
          <button onClick={() => setDrawer({})} title="Nuevo egreso"
            style={{ position: "fixed", right: 32, bottom: FAB_BOTTOM, zIndex: 50, height: FAB_HEIGHT, padding: "0 26px", borderRadius: 99, display: "inline-flex", alignItems: "center", gap: 10, background: "var(--primary)", color: "var(--primary-ink)", fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16.5, cursor: "pointer", boxShadow: "var(--shadow-fab)" }}>
            <Icon name="plus" size={22} stroke={2.6} />Nuevo egreso
          </button>
        </>
      )}

      {drawer !== null && (
        <ExpenseDrawer
          key={drawer.expense?.id ?? "new"}
          expense={drawer.expense}
          categories={categories ?? []}
          today={today}
          canWrite={canWrite}
          onClose={() => setDrawer(null)}
          onSaved={onSaved}
          onCreateCategory={noActiveCategory ? createFirstCategory : undefined}
        />
      )}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </Page>
  );
}

const th = { textAlign: "left", fontSize: 12, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em", padding: "12px 14px" } as const;
const td = { padding: "14px", fontSize: 14.5, color: "var(--text)", borderTop: "1px solid var(--border)" } as const;
/** A row's open button reads as the date it shows; the row around it stays the mouse target. */
const rowButton = { background: "none", border: 0, padding: 0, font: "inherit", color: "inherit", cursor: "pointer", minHeight: 24 } as const;

function ExpensesTable({
  rows, loading, status, activeCount, cancelledCount, onStatus, onOpen, page, pages, total, onPage,
}: {
  rows: readonly Expense[];
  loading: boolean;
  status: StatusFilter;
  activeCount: number;
  cancelledCount: number;
  onStatus: (s: StatusFilter) => void;
  onOpen: (e: Expense) => void;
  page: number;
  pages: number;
  total: number;
  onPage: (p: number) => void;
}): ReactElement {
  const filters: { id: StatusFilter; label: string; count: number }[] = [
    { id: "active", label: "Registrados", count: activeCount },
    { id: "cancelled", label: "Canceladas", count: cancelledCount },
  ];
  const from = total === 0 ? 0 : page * PAGE_SIZE + 1;
  const to = Math.min(total, (page + 1) * PAGE_SIZE);
  return (
    <div style={{ borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
      <div role="tablist" aria-label="Estado" style={{ display: "flex", gap: 6, padding: 12 }}>
        {filters.map((f) => {
          const on = f.id === status;
          return (
            <button key={f.id} role="tab" aria-selected={on} onClick={() => onStatus(f.id)}
              style={{ minHeight: 40, padding: "0 14px", borderRadius: 99, background: on ? "var(--surface-3)" : "transparent", border: "1px solid var(--border)", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 600 }}>
              {`${f.label} · ${f.count}`}
            </button>
          );
        })}
      </div>
      <div style={{ overflowX: "auto" }}>
        <table style={{ width: "100%", borderCollapse: "collapse", minWidth: 640 }}>
          <thead>
            <tr>
              <th style={th}>Fecha</th><th style={th}>Categoría</th><th style={th}>Descripción</th><th style={th}>Forma de pago</th>
              <th style={{ ...th, textAlign: "right" }}>Monto</th><th style={th} aria-label="Comprobante" />
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td style={td} colSpan={6}>Cargando…</td></tr>}
            {!loading && rows.length === 0 && (
              <tr><td style={{ ...td, color: "var(--text-muted)" }} colSpan={6}>{status === "active" ? "No hay egresos registrados este mes." : "No hay egresos cancelados este mes."}</td></tr>
            )}
            {rows.map((e) => (
              /*
                The whole row opens the expense for the mouse; the keyboard and
                assistive tech get a real button in its first cell (ISSUE-T14-021).
                The button has no handler of its own: its click (Enter or Space
                included) bubbles to the row, so it opens once.
              */
              <tr key={e.id} data-testid="expense-row" onClick={() => onOpen(e)}
                style={{ cursor: "pointer", boxShadow: `inset 3px 0 0 ${e.status === "cancelled" ? "var(--red)" : "var(--forest)"}` }}>
                <td style={td}>
                  <button type="button" aria-label={`Abrir egreso: ${e.categoryName}, ${formatCents(e.amountCents)}, ${dayWithYearLabel(e.spentOn)}`} style={rowButton}>
                    {dayWithYearLabel(e.spentOn)}
                  </button>
                </td>
                <td style={td}>{e.categoryName}</td>
                <td style={{ ...td, color: "var(--text-muted)" }}>
                  {e.description !== "" ? e.description : "—"}
                  {e.status === "cancelled" && ` · cancelado: ${e.cancelReason ?? ""}`}
                </td>
                <td style={td}>{METHOD_LABEL[e.method]}</td>
                <td style={{ ...td, textAlign: "right", fontWeight: 700, textDecoration: e.status === "cancelled" ? "line-through" : "none" }}>{formatCents(e.amountCents)}</td>
                <td style={{ ...td, color: "var(--text-faint)" }}>{e.receipt !== undefined && <Icon name="documents" size={18} />}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {total > PAGE_SIZE && (
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "12px 16px", borderTop: "1px solid var(--border)", fontSize: 14, color: "var(--text-muted)" }}>
          <span>{`Mostrando ${from}–${to} de ${total}`}</span>
          <span style={{ display: "flex", gap: 8, alignItems: "center" }}>
            <Btn kind="ghost" disabled={page === 0} onClick={() => onPage(page - 1)} style={{ height: 40 }}>Anterior</Btn>
            {`${page + 1} / ${pages}`}
            <Btn kind="ghost" disabled={page + 1 >= pages} onClick={() => onPage(page + 1)} style={{ height: 40 }}>Siguiente</Btn>
          </span>
        </div>
      )}
    </div>
  );
}

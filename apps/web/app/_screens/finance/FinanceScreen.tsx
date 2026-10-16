"use client";

import { useCallback, useEffect, useMemo, useState, type CSSProperties, type ReactElement } from "react";
import { Avatar } from "../../_design/Brand";
import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { Icon, type IconName } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import type { ColorKey } from "../../_design/colors";
import { DateField, MoneyField, useFocusFirstInvalid } from "../../_design/form";
import { TenderEditor } from "../../_design/payments/TenderEditor";
import { AMOUNT_ERROR } from "../../_design/payments/tenders";
import { useTenders } from "../../_design/payments/useTenders";
import { paymentDateError } from "./CollectDrawer";
import { StudentNotesBeside, useNotesBeside, type NotesAccess } from "../../_design/students/StudentNotesBeside";
import { listCycles, type Cycle } from "../../config/api";
import { getOverdue, listCharges, registerPayment, type Charge } from "../../finance/api";
import { pendingCents } from "../../finance/pending";
import { localISODate } from "../../_lib/localDate";
import { schoolISODate, schoolMonthStart } from "../../_lib/schoolDay";
import { centsToPesosInput, formatCents, pesosToCents } from "../../_lib/money";
import { listStudents, type Student } from "../../students/api";
import { useIntent } from "../../routing/pendingIntent";

import { API_URL } from "../../api-url";
import type { CutAccess } from "./CashCutClosing";
import { CashUpDrawer } from "./CashUpDrawer";
import { IncomePanel } from "./IncomePanel";
import { csvName, downloadCsv, toCsv, type CsvColumn } from "../../_lib/csv";
import { matchesSearch } from "../../_design/textSearch";
import { compareStudentNames, formatStudentName } from "../../_design/students/studentName";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";

/** One key per payment attempt: a retried submit is the same payment, never a second one. */
function newAttemptKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}


// Money crosses the boundary as integer cents; formatting/parsing goes through
// the tested money helpers (locale-aware, cents-exact) — never a naive parseFloat.
const money = (cents: number): string => formatCents(cents);
const AV: ColorKey[] = ["lime", "forest", "orange", "purple", "yellow"];
type VisualStatus = "paid" | "due" | "overdue" | "cancelled";
const STATUS: Record<VisualStatus, { label: string; dot: string; bg: string; fg: string }> = {
  paid: { label: "Pagado", dot: "var(--forest)", bg: "var(--ok-bg)", fg: "var(--ok-fg)" },
  due: { label: "Por vencer", dot: "var(--yellow)", bg: "var(--warn-bg)", fg: "var(--warn-fg)" },
  overdue: { label: "Vencido", dot: "var(--red)", bg: "var(--bad-bg)", fg: "var(--bad-fg)" },
  cancelled: { label: "Cancelado", dot: "var(--text-faint)", bg: "var(--surface-3)", fg: "var(--text-muted)" },
};

const pager = (disabled: boolean): CSSProperties => ({ display: "inline-flex", alignItems: "center", gap: 6, height: 40, padding: "0 16px", borderRadius: 99, fontSize: 14, fontWeight: 600, fontFamily: "var(--font-display)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", cursor: disabled ? "not-allowed" : "pointer", opacity: disabled ? 0.45 : 1 });

interface Row {
  charge: Charge;
  name: string;
  level: string;
  grade: string;
  visual: VisualStatus;
}

function StatusPill({ status }: { status: VisualStatus }): ReactElement {
  const s = STATUS[status];
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 7, padding: "5px 12px 5px 10px", borderRadius: 99, background: s.bg, color: s.fg, fontWeight: 700, fontSize: 13, fontFamily: "var(--font-display)", whiteSpace: "nowrap" }}>
      <span style={{ width: 9, height: 9, borderRadius: 99, background: s.dot, boxShadow: `0 0 0 3px color-mix(in oklab, ${s.dot} 22%, transparent)` }} />{s.label}
    </span>
  );
}

/** What is wrong with the amount paid against one charge, if anything: never more than it still owes. */
export function chargeAmountError(cents: number, remainingCents: number): string | undefined {
  if (!Number.isFinite(cents) || cents <= 0) return AMOUNT_ERROR;
  return cents > remainingCents ? `No puede ser mayor que el saldo del cargo (${money(remainingCents)}).` : undefined;
}

// ── payment slide-over: pay a charge in one panel ──
function PayDrawer({ charge, name, today, notes, onClose, onPaid }: { charge: Charge; name: string; today?: string; notes?: NotesAccess; onClose: () => void; onPaid: () => void }): ReactElement {
  // The pupil's notes on the left, as at the counter (T6).
  const beside = useNotesBeside(charge.studentId, notes);
  const [amount, setAmount] = useState(centsToPesosInput(charge.remainingCents));
  const [amountError, setAmountError] = useState<string | undefined>(undefined);
  // The day the family paid (task 7): not always the day it is keyed.
  const day = today ?? localISODate();
  const [paidOn, setPaidOn] = useState(day);
  // Only a payment dated earlier travels with its date (see CollectDrawer).
  const sentDate = paidOn === day ? "" : paidOn;
  const focusFirstInvalid = useFocusFirstInvalid();
  const [dateError, setDateError] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  // One key per ATTEMPT, minted when the drawer opens and kept across retries.
  // The gateway dedupes on it, so a retry after a network error returns the
  // original payment instead of charging the family twice. This shipped as ""
  // — which the proto documents as "no deduplication (every call posts a new
  // payment)" — so a double click double-charged.
  const [attemptKey] = useState(newAttemptKey);
  // Anything typed makes the drawer dirty: a stray click on the backdrop then
  // stops closing it. Losing a captured amount at counter speed, with a parent
  // waiting, is worse than an extra click on the X.
  // pesosToCents is locale-aware (handles thousands commas) and returns NaN for
  // anything ambiguous — never a truncated parseFloat.
  const cents = pesosToCents(amount);
  // How it is paid, method by method (T8): the rows must add up to the amount.
  const tenders = useTenders(cents);
  const dirty = amount.trim() !== "" || tenders.dirty;

  const submit = async (): Promise<void> => {
    if (saving) return;
    const wrongDate = paymentDateError(paidOn, day);
    const wrongAmount = chargeAmountError(cents, charge.remainingCents);
    setDateError(wrongDate);
    setAmountError(wrongAmount);
    const tendersOk = wrongAmount === undefined && tenders.validate();
    if (wrongDate !== undefined || !tendersOk) {
      focusFirstInvalid();
      return;
    }
    const split = tenders.toTenders();
    const [first] = split;
    setSaving(true);
    setError(undefined);
    try {
      await registerPayment(API_URL, {
        chargeId: charge.id,
        amountCents: cents,
        // The first tender, for a gateway that reads only one method.
        method: first?.method ?? "cash",
        reference: first?.reference ?? "",
        tenders: split,
        idempotencyKey: attemptKey,
        ...(sentDate !== "" ? { paidAt: sentDate } : {}),
      });
      onPaid();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo registrar el pago.");
      setSaving(false);
    }
  };

  return (
    <DrawerShell
      title="Registrar pago"
      subtitle={`${name} · ${charge.description}`}
      icon="card"
      // Con la forma empezada, el velo y Esc dejan de cerrar: perder un pago a
      // medio capturar por un clic perdido es la forma más rápida de que alguien
      // deje de usar la pantalla.
      //
      // `onClose` sigue siendo el de verdad. Pasarle un () => {} apagaba también
      // el ✕ y el botón Cancelar, porque DrawerShell usa esa misma función para
      // las cuatro vías: la protección contra el accidente dejaba a la persona
      // encerrada, que es peor.
      dismissible={!dirty && !beside.dirty}
      size={beside.wide ? "lg" : "md"}
      onClose={onClose}
      footer={
        <>
          <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          <Btn icon="check" onClick={() => void submit()} style={{ height: 52, fontSize: 16 }}>Registrar pago</Btn>
        </>
      }
    >
      <StudentNotesBeside beside={beside} studentId={charge.studentId}>
            <div style={{ display: "flex", justifyContent: "space-between", padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>Saldo del cargo</span>
              <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 18, color: "var(--text)" }}>{money(charge.remainingCents)}</span>
            </div>
            <MoneyField
              label="Monto a pagar"
              value={amount}
              onChange={(v) => {
                setAmount(v);
                if (chargeAmountError(pesosToCents(v), charge.remainingCents) === undefined) setAmountError(undefined);
              }}
              onBlur={() => setAmountError(chargeAmountError(cents, charge.remainingCents))}
              error={amountError}
              help="Lo que la familia paga ahora de este cargo: puede ser un abono, nunca más que su saldo."
              required
            />
            <TenderEditor tenders={tenders} />
            <DateField
              label="Fecha de pago"
              value={paidOn}
              max={day}
              onChange={(v) => {
                setPaidOn(v);
                if (paymentDateError(v, day) === undefined) setDateError(undefined);
              }}
              onBlur={() => setDateError(paymentDateError(paidOn, day))}
              error={dateError}
              help="El día en que la familia pagó. Si capturas tarde, pon ese día: el recargo y el pronto pago se calculan con él."
            />
            {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>}
      </StudentNotesBeside>
    </DrawerShell>
  );
}

/** "2026-09" as a person reads it. */
function monthName(iso: string): string {
  const [year, month] = iso.split("-");
  const names = ["enero", "febrero", "marzo", "abril", "mayo", "junio",
    "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"];
  const name = names[Number(month) - 1];
  return name === undefined ? iso : `${name} ${year}`;
}


/** The columns the school reads in Excel, in the order the table shows them. */
const EXPORT_COLUMNS: readonly CsvColumn<Row>[] = [
  { label: "Alumno", value: (r) => r.name },
  { label: "Nivel", value: (r) => r.level },
  { label: "Grupo", value: (r) => r.grade },
  { label: "Concepto", value: (r) => r.charge.description },
  { label: "Vence", value: (r) => r.charge.dueDate },
  // Pesos with two decimals, not cents: the file is read by a person, and the
  // column is summed in Excel — so deliberately WITHOUT thousands separators,
  // which a spreadsheet would read as text.
  { label: "Importe", value: (r) => centsToPesosInput(r.charge.amountCents) },
  { label: "Descuento", value: (r) => centsToPesosInput(r.charge.discountCents) },
  // T18: Saldo is net of it, so without this column the file would not add up.
  { label: "Beca", value: (r) => centsToPesosInput(r.charge.scholarshipCents ?? 0) },
  { label: "Pagado", value: (r) => centsToPesosInput(r.charge.paidCents) },
  { label: "Saldo", value: (r) => centsToPesosInput(r.charge.remainingCents) },
  { label: "Estado", value: (r) => STATUS[r.visual].label },
];

export interface FinanceScreenProps {
  onBack: () => void;
  /** The office's day (aaaa-mm-dd); injected by tests. Defaults to today, locally. */
  today?: string;
  /** What the caller may do with a pupil's notes, shown beside Registrar pago (T6). */
  notes?: NotesAccess;
  /** payments:edit: may correct a registered payment from the day's cut (T9). */
  canCorrectPayments?: boolean;
  /** T10: expenses and closing the day in the cut. Absent: module_expenses is off. */
  cutAccess?: CutAccess;
}

export function FinanceScreen({ onBack, today: officeDay, notes, canCorrectPayments = false, cutAccess }: FinanceScreenProps): ReactElement {
  const [charges, setCharges] = useState<Charge[]>([]);
  const [overdueIds, setOverdueIds] = useState<Set<string>>(new Set());
  const [studentMap, setStudentMap] = useState<Map<string, Student>>(new Map());
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [loading, setLoading] = useState(true);
  const intent = useIntent("finance");
  const [tab, setTab] = useState<"all" | "overdue" | "due" | "paid">(intent === "overdue" ? "overdue" : "all");
  const [q, setQ] = useState("");
  // Which month's charges to look at. The school reads its debt a month at a
  // time — "¿quién debe septiembre?" — and a year of rows on one screen is not
  // an answer to that.
  const [month, setMonth] = useState("todos");
  const [pay, setPay] = useState<Row | null>(null);
  // Bumped by every payment so the period's income is read again (QA ISSUE-001).
  const [paidCount, setPaidCount] = useState(0);
  // The counter: pick a pupil, then take whatever the parent hands over. Kept
  // separate from `pay`, which is the older per-charge path.
  /*
   * Aquí vivían `picking` y `collecting`, con su buscador de alumno y su cajón
   * de cobro. `setPicking(true)` NO SE LLAMABA EN NINGÚN LADO, así que las dos
   * pantallas eran inalcanzables: el primer intento de poner la ventanilla en un
   * rincón de Finanzas, abandonado cuando la ventanilla se mudó a su propia
   * pantalla y nunca borrado.
   *
   * La decisión ya estaba tomada y escrita en `PaymentsScreen`: "deliberadamente
   * NO es un rincón de Finanzas. Finanzas contesta la pregunta de la dirección
   * —qué entró, qué se debe— y esto contesta la del mostrador: hay un papá ahí
   * parado con dinero. Distinta gente, distintas horas, distinta pantalla."
   *
   * Cobrar un cargo suelto desde aquí sigue existiendo: es `PayDrawer`.
   */
  const [cashUp, setCashUp] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [pageSize, setPageSize] = useState(20);
  const [page, setPage] = useState(0);

  const say = (m: string): void => { setToast(m); setTimeout(() => setToast(null), 2600); };

  const load = useCallback((cycleId: string): void => {
    setLoading(true);
    Promise.all([listCharges(API_URL, { cycleId }), getOverdue(API_URL, cycleId)])
      .then(([cs, ov]) => { setCharges(cs); setOverdueIds(new Set(ov.map((c) => c.id))); })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    listStudents(API_URL).then((ss) => setStudentMap(new Map(ss.map((s) => [s.id, s])))).catch(() => {});
    listCycles(API_URL).then((cs) => {
      setCycles(cs);
      const active = cs.find((c) => c.status === "active") ?? cs[0];
      if (active !== undefined) load(active.id);
      else setLoading(false);
    }).catch(() => setLoading(false));
  }, [load]);

  const visual = useCallback((c: Charge): VisualStatus => (c.status === "cancelled" ? "cancelled" : c.remainingCents <= 0 ? "paid" : overdueIds.has(c.id) ? "overdue" : "due"), [overdueIds]);

  const rows: Row[] = useMemo(() => charges.map((c) => {
    const s = studentMap.get(c.studentId);
    return { charge: c, name: s !== undefined ? formatStudentName(s) : c.studentId, level: s?.level ?? "", grade: s?.gradeGroup ?? "", visual: visual(c) };
  }), [charges, studentMap, visual]);

  const counts = useMemo(() => ({
    all: rows.length,
    overdue: rows.filter((r) => r.visual === "overdue").length,
    due: rows.filter((r) => r.visual === "due").length,
    paid: rows.filter((r) => r.visual === "paid").length,
  }), [rows]);
  const totals = useMemo(() => ({
    collected: charges.reduce((a, c) => a + c.paidCents, 0),
    // A cancelled (voided) charge is never owed, so it must not count as pending debt.
    pending: pendingCents(charges),
  }), [charges]);

  // The months the cycle actually has charges in, newest last, as YYYY-MM.
  const months = useMemo(
    () => [...new Set(charges.map((c) => c.dueDate.slice(0, 7)))].sort(),
    [charges],
  );

  const filtered = useMemo(() => {
    return rows
      .filter((r) => (tab === "all" || r.visual === tab)
        && (month === "todos" || r.charge.dueDate.startsWith(month))
        && matchesSearch(q, r.name, r.charge.description))
      .sort((a, b) => compareStudentNames(a.name, b.name));
  }, [rows, tab, month, q]);

  useEffect(() => { setPage(0); }, [tab, month, q, pageSize]);
  const total = filtered.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const cur = Math.min(page, pageCount - 1);
  const paged = filtered.slice(cur * pageSize, cur * pageSize + pageSize);
  const from = total === 0 ? 0 : cur * pageSize + 1;
  const to = Math.min(total, (cur + 1) * pageSize);

  const cycleName = cycles.find((c) => c.status === "active")?.name ?? cycles[0]?.name ?? "—";
  const activeCycleId = cycles.find((c) => c.status === "active")?.id ?? cycles[0]?.id;
  // "Exportar esta vista" means the list on screen: the tab picked and the
  // search typed, not everything the school has ever charged. Building it from
  // `filtered` is what makes the file match what the person is looking at.
  const exportView = (): void => {
    downloadCsv(csvName("adeudos", new Date()), toCsv(EXPORT_COLUMNS, filtered));
    say(`${filtered.length} renglones exportados`);
  };

  const onPaid = (): void => { setPay(null); setPaidCount((n) => n + 1); if (activeCycleId !== undefined) load(activeCycleId); say("Pago registrado"); };

  const tabs: [typeof tab, string][] = [["all", "Todos"], ["overdue", "Vencidos"], ["due", "Por vencer"], ["paid", "Pagados"]];
  const stats: { icon: IconName; c: string; label: string; val: string; sub: string }[] = [
    { icon: "trend", c: "forest", label: "Cobrado", val: money(totals.collected), sub: `Ciclo ${cycleName}` },
    { icon: "clock", c: "orange", label: "Por cobrar", val: money(totals.pending), sub: `${counts.due + counts.overdue} cargos con saldo` },
    { icon: "warning", c: "red", label: "Vencidos", val: String(counts.overdue), sub: "Requieren recordatorio" },
    { icon: "check", c: "lime", label: "Pagados", val: String(counts.paid), sub: "Sin saldo" },
  ];

  return (
    <Page>
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}>
        <Icon name="back" size={18} stroke={2.4} />Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 16, marginBottom: 24 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <ModuleHeaderIcon icon="finance" color="orange" shrinks />
          <div>
            <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Finanzas</h1>
            <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Ciclo {cycleName} · Ingresos recibidos y adeudos</p>
          </div>
        </div>
        <div style={{ display: "flex", gap: 12 }}>
          <Btn kind="ghost" icon="download" onClick={() => exportView()}>Exportar</Btn>
          <Btn
            kind="ghost"
            icon="finance"
            // Each opening mounts the drawer afresh, on today: opening the cut
            // is closing the day.
            onClick={() => setCashUp(true)}
          >
            Corte de caja
          </Btn>
        </div>
      </div>

      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 26 }}>
        {stats.map((k, i) => (
          <div key={i} style={{ flex: "1 1 200px", minWidth: 0, display: "flex", alignItems: "center", gap: 14, padding: "18px 20px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
            <div style={{ width: 46, height: 46, borderRadius: 13, flexShrink: 0, display: "grid", placeItems: "center", background: `color-mix(in oklab, var(--${k.c}) 16%, var(--surface))`, color: `var(--${k.c})` }}><Icon name={k.icon} size={24} /></div>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600, whiteSpace: "nowrap" }}>{k.label}</div>
              <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 22, color: "var(--text)", lineHeight: 1.2, whiteSpace: "nowrap" }}>{k.val}</div>
              <div style={{ fontSize: 12, color: "var(--text-faint)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{k.sub}</div>
            </div>
          </div>
        ))}
      </div>

      {/* What came IN, before what is owed. The director's first question is
          how the month is going, and it used to take opening a drawer. */}
      <div className="fade-up" style={{ marginBottom: 26 }}>
        <IncomePanel key={paidCount} from={schoolMonthStart()} to={schoolISODate()} />
      </div>

      <h2 className="fade-up" style={{ fontSize: 20, fontWeight: 700, color: "var(--text)", marginBottom: 14 }}>Adeudos</h2>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap", marginBottom: 18 }}>
        <div style={{ display: "flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)" }}>
          {tabs.map(([id, label]) => {
            const on = tab === id;
            return (
              <button key={id} aria-pressed={on} onClick={() => setTab(id)} style={{ display: "inline-flex", alignItems: "center", gap: 8, height: 40, padding: "0 16px", borderRadius: 99, cursor: "pointer", background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 600, fontSize: 14, fontFamily: "var(--font-display)", boxShadow: on ? "var(--shadow-card)" : "none" }}>
                {label}<span style={{ minWidth: 22, height: 20, padding: "0 6px", borderRadius: 99, background: on ? "var(--surface-3)" : "var(--surface)", color: "var(--text-muted)", fontSize: 12, display: "grid", placeItems: "center" }}>{counts[id]}</span>
              </button>
            );
          })}
        </div>
        {months.length > 1 && (
          <select
            aria-label="Mes"
            value={month}
            onChange={(e) => setMonth(e.target.value)}
            style={{ height: 48, padding: "0 14px", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 14.5, cursor: "pointer" }}>
            <option value="todos">Todos los meses</option>
            {months.map((m) => <option key={m} value={m}>{monthName(m)}</option>)}
          </select>
        )}
        <div style={{ position: "relative", flex: 1, minWidth: 230, maxWidth: 360 }}>
          <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}><Icon name="search" size={19} /></span>
          <input value={q} onChange={(e) => setQ(e.target.value)} aria-label="Buscar" placeholder="Buscar alumno o concepto…" style={{ width: "100%", height: 48, paddingLeft: 46, paddingRight: 16, borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 15, outline: "none" }} />
        </div>
      </div>

      <div className="fade-up" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
        <div style={{ display: "grid", gridTemplateColumns: "2.2fr 1fr 1.3fr 0.9fr 1.1fr auto", gap: 16, padding: "14px 24px 12px", borderBottom: "1px solid var(--border)", fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em" }}>
          <div>Alumno</div><div>Nivel</div><div>Concepto</div><div>Vence</div><div>Saldo · Estado</div><div style={{ width: 148 }} />
        </div>
        <div style={{ padding: 8 }}>
          {loading ? (
            <div style={{ padding: "56px", textAlign: "center", color: "var(--text-muted)" }}>Cargando…</div>
          ) : filtered.length === 0 ? (
            <div style={{ padding: "56px 20px", textAlign: "center" }}>
              <div style={{ width: 70, height: 70, borderRadius: 20, margin: "0 auto 16px", display: "grid", placeItems: "center", background: "var(--ok-bg)", color: "var(--forest)" }}><Icon name="check" size={36} stroke={2.4} /></div>
              <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19, color: "var(--text)" }}>¡Todo en orden!</div>
              <div style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 6 }}>No hay cargos en esta vista.</div>
            </div>
          ) : (
            paged.map((r, i) => <FinRow key={r.charge.id} r={r} i={i} onPay={() => setPay(r)} />)
          )}
        </div>
        {!loading && total > 0 && (
          <div style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap", padding: "14px 24px", borderTop: "1px solid var(--border)" }}>
            <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>Mostrando <b style={{ color: "var(--text)" }}>{from}–{to}</b> de {total}</span>
            <div style={{ flex: 1 }} />
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span style={{ fontSize: 13, color: "var(--text-faint)" }}>Mostrar</span>
              <div style={{ display: "flex", gap: 4, padding: 4, background: "var(--surface-3)", borderRadius: 99, border: "1px solid var(--border)" }}>
                {[20, 50, 100].map((n) => { const on = pageSize === n; return <button key={n} onClick={() => setPageSize(n)} style={{ height: 32, minWidth: 42, padding: "0 12px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer", background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", boxShadow: on ? "var(--shadow-card)" : "none" }}>{n}</button>; })}
              </div>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <button onClick={() => setPage((p) => Math.max(0, p - 1))} disabled={cur === 0} aria-label="Anterior" style={pager(cur === 0)}><Icon name="back" size={16} stroke={2.4} />Anterior</button>
              <span style={{ fontSize: 13.5, color: "var(--text-muted)", fontFamily: "var(--font-display)", fontWeight: 600, minWidth: 52, textAlign: "center" }}>{cur + 1} / {pageCount}</span>
              <button onClick={() => setPage((p) => Math.min(pageCount - 1, p + 1))} disabled={cur >= pageCount - 1} aria-label="Siguiente" style={pager(cur >= pageCount - 1)}>Siguiente<Icon name="chevron" size={16} stroke={2.4} /></button>
            </div>
          </div>
        )}
      </div>

      {pay !== null && <PayDrawer charge={pay.charge} name={pay.name} today={officeDay} notes={notes} onClose={() => setPay(null)} onPaid={onPaid} />}
      {cashUp && <CashUpDrawer today={officeDay} canCorrect={canCorrectPayments} cutAccess={cutAccess} onClose={() => setCashUp(false)} />}
      {toast !== null && (
        <div role="status" style={{ position: "fixed", bottom: 28, left: "50%", transform: "translateX(-50%)", zIndex: 80, display: "flex", alignItems: "center", gap: 11, padding: "14px 22px", borderRadius: "var(--r-pill)", background: "var(--forest)", color: "#fff", fontWeight: 600, fontSize: 15, boxShadow: "var(--shadow-pop)", animation: "pop .25s var(--ease) both" }}>
          <Icon name="check" size={20} stroke={2.6} />{toast}
        </div>
      )}
    </Page>
  );
}

function FinRow({ r, i, onPay }: { r: Row; i: number; onPay: () => void }): ReactElement {
  const [hov, setHov] = useState(false);
  const s = STATUS[r.visual];
  return (
    <div onMouseEnter={() => setHov(true)} onMouseLeave={() => setHov(false)} style={{ display: "grid", gridTemplateColumns: "2.2fr 1fr 1.3fr 0.9fr 1.1fr auto", alignItems: "center", gap: 16, padding: "14px 24px", borderRadius: "var(--r-md)", position: "relative", background: hov ? "var(--surface-2)" : "transparent", transition: "background .15s var(--ease)" }}>
      <span style={{ position: "absolute", left: 4, top: 10, bottom: 10, width: 4, borderRadius: 99, background: s.dot }} />
      <div style={{ display: "flex", alignItems: "center", gap: 13, minWidth: 0, paddingLeft: 8 }}>
        <Avatar name={r.name} color={AV[i % AV.length]} size={42} />
        <div style={{ minWidth: 0 }}>
          <div style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{r.name}</div>
          <div style={{ fontSize: 12.5, color: "var(--text-faint)", fontFamily: "monospace" }}>{r.charge.studentId.slice(0, 8)}</div>
        </div>
      </div>
      <div style={{ fontSize: 14, color: "var(--text-muted)", textTransform: "capitalize" }}>{r.level}<div style={{ fontSize: 12.5, color: "var(--text-faint)" }}>{r.grade}</div></div>
      <div style={{ fontSize: 14, color: "var(--text-muted)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{r.charge.description}</div>
      <div style={{ fontSize: 14, color: r.visual === "overdue" ? "var(--bad-fg)" : "var(--text-muted)", fontWeight: r.visual === "overdue" ? 600 : 400 }}>{r.charge.dueDate}</div>
      <div>
        <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16, color: r.charge.remainingCents > 0 ? "var(--text)" : "var(--text-faint)" }}>{money(r.charge.remainingCents)}</div>
        <div style={{ marginTop: 4 }}><StatusPill status={r.visual} /></div>
      </div>
      <div style={{ display: "flex", justifyContent: "flex-end", width: 148 }}>
        {r.visual === "paid" ? (
          <span style={{ display: "inline-flex", alignItems: "center", gap: 6, color: "var(--forest)", fontSize: 13.5, fontWeight: 600 }}><Icon name="check" size={18} stroke={2.5} />Al corriente</span>
        ) : r.visual === "cancelled" ? (
          <span style={{ color: "var(--text-faint)", fontSize: 13.5 }}>Cancelado</span>
        ) : (
          <button onClick={onPay} style={{ display: "inline-flex", alignItems: "center", gap: 7, height: 40, padding: "0 16px", borderRadius: 99, cursor: "pointer", background: "var(--primary)", color: "var(--primary-ink)", fontFamily: "var(--font-display)", fontWeight: 600, fontSize: 14, boxShadow: "var(--shadow-primary-sm)" }}><Icon name="card" size={17} />Registrar pago</button>
        )}
      </div>
    </div>
  );
}

"use client";

import { useMemo, useState, type ReactElement } from "react";
import { Avatar } from "../_design/Brand";
import { Btn } from "../_design/Controls";
import { Icon, type IconName } from "../_design/Icon";
import { COLOR, type ColorKey } from "../_design/colors";
import {
  FIN_SUMMARY,
  LEVELS,
  STUDENTS,
  type StudentRow,
  type StudentStatus
} from "../_design/data";

interface StatusStyle {
  label: string;
  dot: string;
  bg: string;
  fg: string;
}

const STATUS: Record<StudentStatus, StatusStyle> = {
  paid: { label: "Pagado", dot: "var(--forest)", bg: "var(--ok-bg)", fg: "var(--ok-fg)" },
  due: { label: "Por vencer", dot: "var(--yellow)", bg: "var(--warn-bg)", fg: "var(--warn-fg)" },
  overdue: { label: "Vencido", dot: "var(--red)", bg: "var(--bad-bg)", fg: "var(--bad-fg)" }
};

const money = (n: number): string => "$" + n.toLocaleString("es-MX");

const ROW_GRID = "2.4fr 1fr 1.3fr 1fr 1.1fr auto";
const AV_COLORS: readonly ColorKey[] = ["lime", "forest", "orange", "purple", "yellow"];

type TabId = "all" | StudentStatus;
const TABS: ReadonlyArray<[TabId, string]> = [
  ["all", "Todos"],
  ["overdue", "Vencidos"],
  ["due", "Por vencer"],
  ["paid", "Pagados"]
];

function StatusPill({ s }: { s: StudentStatus }): ReactElement {
  const st = STATUS[s];
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 7,
        padding: "5px 12px 5px 10px",
        borderRadius: 99,
        background: st.bg,
        color: st.fg,
        fontWeight: 700,
        fontSize: 13,
        fontFamily: "var(--font-display)",
        whiteSpace: "nowrap"
      }}
    >
      <span
        style={{
          width: 9,
          height: 9,
          borderRadius: 99,
          background: st.dot,
          boxShadow: `0 0 0 3px color-mix(in oklab, ${st.dot} 22%, transparent)`
        }}
      />
      {st.label}
    </span>
  );
}

function FinRow({ st, i, onPay }: { st: StudentRow; i: number; onPay: (id: string) => void }): ReactElement {
  const [hov, setHov] = useState(false);
  const sc = STATUS[st.status];
  return (
    <div
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      style={{
        display: "grid",
        gridTemplateColumns: ROW_GRID,
        alignItems: "center",
        gap: 16,
        padding: "14px 20px",
        borderRadius: "var(--r-md)",
        position: "relative",
        background: hov ? "var(--surface-2)" : "transparent",
        transition: "background .15s var(--ease)"
      }}
    >
      <span style={{ position: "absolute", left: 0, top: 10, bottom: 10, width: 4, borderRadius: 99, background: sc.dot }} />
      {/* student */}
      <div style={{ display: "flex", alignItems: "center", gap: 13, minWidth: 0, paddingLeft: 8 }}>
        <Avatar name={st.name} color={AV_COLORS[i % AV_COLORS.length]} size={42} />
        <div style={{ minWidth: 0 }}>
          <div style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
            {st.name}
          </div>
          <div style={{ fontSize: 13, color: "var(--text-faint)" }}>{st.id}</div>
        </div>
      </div>
      {/* level */}
      <div style={{ fontSize: 14, color: "var(--text-muted)" }}>
        {st.level}
        <div style={{ fontSize: 12.5, color: "var(--text-faint)" }}>{st.grade}</div>
      </div>
      {/* concept */}
      <div style={{ fontSize: 14, color: "var(--text-muted)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
        {st.concept}
      </div>
      {/* due */}
      <div
        style={{
          fontSize: 14,
          color: st.status === "overdue" ? "var(--bad-fg)" : "var(--text-muted)",
          fontWeight: st.status === "overdue" ? 600 : 400
        }}
      >
        {st.due}
      </div>
      {/* amount + status */}
      <div>
        <div
          style={{
            fontFamily: "var(--font-display)",
            fontWeight: 700,
            fontSize: 16,
            color: st.amount ? "var(--text)" : "var(--text-faint)"
          }}
        >
          {st.amount ? money(st.amount) : "—"}
        </div>
        <div style={{ marginTop: 4 }}>
          <StatusPill s={st.status} />
        </div>
      </div>
      {/* action */}
      <div style={{ display: "flex", justifyContent: "flex-end", width: 148 }}>
        {st.status === "paid" ? (
          <span style={{ display: "inline-flex", alignItems: "center", gap: 6, color: "var(--forest)", fontSize: 13.5, fontWeight: 600 }}>
            <Icon name="check" size={18} stroke={2.5} />
            Al corriente
          </span>
        ) : (
          <button
            onClick={() => onPay(st.id)}
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 7,
              height: 40,
              padding: "0 16px",
              borderRadius: 99,
              background: "var(--primary)",
              color: "var(--primary-ink)",
              fontFamily: "var(--font-display)",
              fontWeight: 600,
              fontSize: 14,
              boxShadow: "0 3px 10px color-mix(in oklab, var(--lime) 36%, transparent)",
              transition: "filter .15s"
            }}
            onMouseEnter={(e) => (e.currentTarget.style.filter = "brightness(1.05)")}
            onMouseLeave={(e) => (e.currentTarget.style.filter = "none")}
          >
            <Icon name="card" size={17} />
            Registrar pago
          </button>
        )}
      </div>
    </div>
  );
}

interface SummaryCard {
  icon: IconName;
  c: ColorKey;
  label: string;
  val: string;
  sub: string;
}

export interface FinancesProps {
  onBack: () => void;
}

export function Finances({ onBack }: FinancesProps): ReactElement {
  const [rows, setRows] = useState<readonly StudentRow[]>(STUDENTS);
  const [tab, setTab] = useState<TabId>("all");
  const [level, setLevel] = useState<string>("Todos");
  const [q, setQ] = useState("");
  const [toast, setToast] = useState<string | null>(null);

  const pay = (id: string): void => {
    const student = rows.find((r) => r.id === id);
    setRows((rs) => rs.map((r) => (r.id === id ? { ...r, status: "paid", amount: 0, due: "—" } : r)));
    if (student) {
      setToast(`Pago registrado · ${student.name.split(" ")[0]}`);
      window.setTimeout(() => setToast(null), 2600);
    }
  };

  const counts = useMemo(
    () => ({
      all: rows.length,
      paid: rows.filter((r) => r.status === "paid").length,
      due: rows.filter((r) => r.status === "due").length,
      overdue: rows.filter((r) => r.status === "overdue").length
    }),
    [rows]
  );

  const filtered = useMemo(
    () =>
      rows.filter(
        (r) =>
          (tab === "all" || r.status === tab) &&
          (level === "Todos" || r.level === level) &&
          (q === "" || r.name.toLowerCase().includes(q.toLowerCase()) || r.id.toLowerCase().includes(q.toLowerCase()))
      ),
    [rows, tab, level, q]
  );

  const pendingTotal = useMemo(
    () => rows.filter((r) => r.status !== "paid").reduce((a, b) => a + b.amount, 0),
    [rows]
  );

  const summary: readonly SummaryCard[] = [
    { icon: "trend", c: "forest", label: "Cobrado en el mes", val: money(FIN_SUMMARY.collected), sub: `${FIN_SUMMARY.paidPct}% de lo esperado` },
    { icon: "clock", c: "orange", label: "Pendiente por cobrar", val: money(pendingTotal), sub: `${counts.due + counts.overdue} alumnos` },
    { icon: "warning", c: "red", label: "Vencidos", val: String(counts.overdue), sub: "Requieren recordatorio" },
    { icon: "check", c: "lime", label: "Al corriente", val: String(counts.paid), sub: "Sin adeudos" }
  ];

  return (
    <div style={{ maxWidth: 1320, margin: "0 auto", padding: "24px 28px 64px" }}>
      {/* breadcrumb + title */}
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>
      <div
        className="fade-up"
        style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 16, marginBottom: 24 }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <div
            style={{
              width: 56,
              height: 56,
              borderRadius: 18,
              display: "grid",
              placeItems: "center",
              background: "var(--orange)",
              color: "#fff",
              boxShadow: "0 6px 16px color-mix(in oklab, var(--orange) 38%, transparent)"
            }}
          >
            <Icon name="finance" size={30} />
          </div>
          <div>
            <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Finanzas y Adeudos</h1>
            <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Junio 2025 · Colegiaturas y conceptos</p>
          </div>
        </div>
        <div style={{ display: "flex", gap: 12 }}>
          <Btn kind="ghost" icon="download">
            Exportar
          </Btn>
          <Btn icon="plus">Registrar cobro</Btn>
        </div>
      </div>

      {/* summary */}
      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 26 }}>
        {summary.map((k, idx) => {
          const c = COLOR[k.c];
          return (
            <div
              key={idx}
              style={{
                flex: "1 1 200px",
                minWidth: 0,
                display: "flex",
                alignItems: "center",
                gap: 14,
                padding: "18px 20px",
                borderRadius: "var(--r-lg)",
                background: "var(--surface)",
                border: "1px solid var(--border)",
                boxShadow: "var(--shadow-card)"
              }}
            >
              <div
                style={{
                  width: 46,
                  height: 46,
                  borderRadius: 13,
                  flexShrink: 0,
                  display: "grid",
                  placeItems: "center",
                  background: `color-mix(in oklab, ${c.v} 16%, var(--surface))`,
                  color: c.v
                }}
              >
                <Icon name={k.icon} size={24} />
              </div>
              <div style={{ minWidth: 0 }}>
                <div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600, whiteSpace: "nowrap" }}>{k.label}</div>
                <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 22, color: "var(--text)", lineHeight: 1.2, whiteSpace: "nowrap" }}>
                  {k.val}
                </div>
                <div style={{ fontSize: 12, color: "var(--text-faint)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{k.sub}</div>
              </div>
            </div>
          );
        })}
      </div>

      {/* controls */}
      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap", marginBottom: 18 }}>
        <div
          role="tablist"
          aria-label="Filtrar por estado"
          style={{ display: "flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)" }}
        >
          {TABS.map(([id, label]) => {
            const on = tab === id;
            return (
              <button
                key={id}
                role="tab"
                aria-selected={on}
                onClick={() => setTab(id)}
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 8,
                  height: 40,
                  padding: "0 16px",
                  borderRadius: 99,
                  background: on ? "var(--surface)" : "transparent",
                  color: on ? "var(--text)" : "var(--text-muted)",
                  fontWeight: 600,
                  fontSize: 14,
                  fontFamily: "var(--font-display)",
                  boxShadow: on ? "var(--shadow-card)" : "none",
                  transition: "all .15s var(--ease)"
                }}
              >
                {label}
                <span
                  style={{
                    minWidth: 22,
                    height: 20,
                    padding: "0 6px",
                    borderRadius: 99,
                    background: on ? "var(--surface-3)" : "var(--surface)",
                    color: "var(--text-muted)",
                    fontSize: 12,
                    display: "grid",
                    placeItems: "center"
                  }}
                >
                  {counts[id]}
                </span>
              </button>
            );
          })}
        </div>
        <div style={{ position: "relative", flex: 1, minWidth: 220, maxWidth: 340 }}>
          <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}>
            <Icon name="search" size={19} />
          </span>
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Buscar alumno o matrícula…"
            aria-label="Buscar alumno o matrícula"
            style={{
              width: "100%",
              height: 48,
              paddingLeft: 46,
              paddingRight: 16,
              borderRadius: "var(--r-pill)",
              border: "1px solid var(--border)",
              background: "var(--surface)",
              color: "var(--text)",
              fontSize: 15,
              outline: "none"
            }}
          />
        </div>
        <div style={{ display: "flex", gap: 7, flexWrap: "wrap" }}>
          {["Todos", ...LEVELS].map((l) => {
            const on = level === l;
            return (
              <button
                key={l}
                aria-pressed={on}
                onClick={() => setLevel(l)}
                style={{
                  height: 40,
                  padding: "0 14px",
                  borderRadius: 99,
                  fontSize: 13.5,
                  fontWeight: 600,
                  fontFamily: "var(--font-display)",
                  border: `1.5px solid ${on ? "var(--forest)" : "var(--border)"}`,
                  background: on ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
                  color: on ? "var(--forest)" : "var(--text-muted)",
                  transition: "all .15s"
                }}
              >
                {l}
              </button>
            );
          })}
        </div>
      </div>

      {/* table */}
      <div
        className="fade-up"
        style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}
      >
        <div
          style={{
            display: "grid",
            gridTemplateColumns: ROW_GRID,
            gap: 16,
            padding: "14px 20px 12px",
            borderBottom: "1px solid var(--border)",
            fontSize: 12.5,
            fontWeight: 700,
            color: "var(--text-faint)",
            textTransform: "uppercase",
            letterSpacing: ".04em"
          }}
        >
          <div style={{ paddingLeft: 8 }}>Alumno</div>
          <div>Nivel</div>
          <div>Concepto</div>
          <div>Vence</div>
          <div>Monto · Estado</div>
          <div style={{ width: 148 }} />
        </div>
        <div style={{ padding: "8px" }}>
          {filtered.length === 0 ? (
            <div style={{ padding: "56px 20px", textAlign: "center" }}>
              <div
                style={{
                  width: 70,
                  height: 70,
                  borderRadius: 20,
                  margin: "0 auto 16px",
                  display: "grid",
                  placeItems: "center",
                  background: "var(--ok-bg)",
                  color: "var(--forest)"
                }}
              >
                <Icon name="check" size={36} stroke={2.4} />
              </div>
              <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19, color: "var(--text)" }}>¡Todo en orden!</div>
              <div style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 6 }}>No hay alumnos en esta vista. Selecciona otro filtro.</div>
            </div>
          ) : (
            filtered.map((st, i) => <FinRow key={st.id} st={st} i={i} onPay={pay} />)
          )}
        </div>
      </div>

      {/* toast */}
      {toast && (
        <div
          role="status"
          style={{
            position: "fixed",
            bottom: 28,
            left: "50%",
            transform: "translateX(-50%)",
            zIndex: 60,
            display: "flex",
            alignItems: "center",
            gap: 11,
            padding: "14px 22px",
            borderRadius: "var(--r-pill)",
            background: "var(--forest)",
            color: "#fff",
            fontWeight: 600,
            fontSize: 15,
            boxShadow: "var(--shadow-pop)",
            animation: "pop .25s var(--ease) both"
          }}
        >
          <Icon name="check" size={20} stroke={2.6} />
          {toast}
        </div>
      )}
    </div>
  );
}

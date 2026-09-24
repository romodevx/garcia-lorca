"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type ReactElement } from "react";
import { Avatar } from "../../_design/Brand";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { listStudents, type Student } from "../../students/api";
import { getExpected } from "../../finance/api";
import { API_URL } from "../../api-url";
import { formatCents } from "../../_lib/money";
import { CashUpDrawer } from "../finance/CashUpDrawer";
import { CollectDrawer, type CollectStudent } from "../finance/CollectDrawer";

const search: CSSProperties = {
  width: "100%", height: 60, paddingLeft: 52, paddingRight: 18,
  borderRadius: "var(--r-pill)", border: "1px solid var(--border)",
  background: "var(--surface)", color: "var(--text)", fontSize: 18, outline: "none",
};

/** How many names to show at once. Longer than this is scrolled, not read. */
const MAX_RESULTS = 12;

export interface PaymentsScreenProps {
  onBack: () => void;
  /** finance:write: the counter may grant the late early-payment reward. */
  canGrantDiscounts?: boolean;
}

/**
 * Registro de pagos: the counter, on its own.
 *
 * It is deliberately not a corner of Finanzas. Finanzas answers the director's
 * question — what came in, what is owed — and this answers the front desk's: a
 * parent is standing there with money. Different people, different hours,
 * different screen.
 *
 * The whole flow is one question: which pupil? Everything else happens in the
 * drawer.
 */
export function PaymentsScreen({ onBack, canGrantDiscounts = false }: PaymentsScreenProps): ReactElement {
  const [students, setStudents] = useState<Student[]>([]);
  // Three states, not two. "Empty" and "we could not ask" look the same on
  // screen and mean opposite things: one says the pupil is not registered, the
  // other says we do not know. Telling the office the first when the truth is
  // the second sends somebody looking for a registration problem that is not
  // there — which is exactly what happened when the students service was down.
  const [roll, setRoll] = useState<"loading" | "ready" | "failed">("loading");
  const [q, setQ] = useState("");
  const [collecting, setCollecting] = useState<CollectStudent | null>(null);
  const [cashUp, setCashUp] = useState(false);
  const [owed, setOwed] = useState<Map<string, number>>(new Map());
  const [toast, setToast] = useState<string | null>(null);

  const loadRoll = useCallback(() => {
    setRoll("loading");
    listStudents(API_URL)
      .then((rows) => { setStudents(rows); setRoll("ready"); })
      .catch(() => setRoll("failed"));
  }, []);

  useEffect(loadRoll, [loadRoll]);

  const matches = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (needle === "") {
      return [];
    }
    return students
      .filter((s) => `${s.firstName} ${s.lastName}`.toLowerCase().includes(needle) || s.curp.toLowerCase().includes(needle))
      .slice(0, MAX_RESULTS);
  }, [students, q]);

  // What each visible pupil owes today, so the cashier sees it before opening
  // anything. Only for the handful on screen: asking for four hundred would
  // make typing a name slow.
  //
  // The "already asked" set is a ref, not state, and deliberately: the first
  // version listed `owed` among this effect's dependencies and then wrote to it,
  // so every answer re-ran the effect, threw the other requests away and started
  // them again. A dozen names became dozens of round trips.
  const asked = useRef<Set<string>>(new Set());
  useEffect(() => {
    let live = true;
    for (const s of matches) {
      if (asked.current.has(s.id)) {
        continue;
      }
      asked.current.add(s.id);
      getExpected(API_URL, s.id)
        .then((e) => { if (live) { setOwed((prev) => new Map(prev).set(s.id, e.amountCents)); } })
        .catch(() => {
          // Unknown, not zero. Asked again next time the name comes up rather
          // than showing a balance we never got.
          asked.current.delete(s.id);
        });
    }
    return () => { live = false; };
  }, [matches]);

  const say = (m: string): void => { setToast(m); setTimeout(() => setToast(null), 2600); };

  return (
    <Page variant="reading">
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}>
        <Icon name="back" size={18} stroke={2.4} />Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 26 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--forest) 38%, transparent)" }}>
          <Icon name="card" size={30} />
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Registro de pagos</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Busca al alumno y captura lo que entregó</p>
        </div>
        {/* The cashier closes the day here: the Cobranza role (task 9) does
            not open Finanzas, where the cut also lives. */}
        <Btn kind="ghost" icon="finance" onClick={() => setCashUp(true)}>Corte de caja</Btn>
      </div>

      <div className="fade-up" style={{ position: "relative", marginBottom: 18 }}>
        <span style={{ position: "absolute", left: 20, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}>
          <Icon name="search" size={22} />
        </span>
        <input autoFocus value={q} onChange={(e) => setQ(e.target.value)} aria-label="Buscar alumno" placeholder="Nombre o CURP del alumno…" style={search} />
      </div>

      {roll === "failed" && (
        <div role="alert" style={{ padding: "22px 24px", borderRadius: "var(--r-lg)", background: "var(--bad-bg)", border: "1px solid var(--border)", textAlign: "center" }}>
          <p style={{ color: "var(--bad-fg)", fontSize: 15, marginBottom: 12 }}>
            No pudimos cargar la lista de alumnos. No es que no existan — no la pudimos leer.
          </p>
          <Btn kind="ghost" onClick={loadRoll}>Reintentar</Btn>
        </div>
      )}
      {roll === "loading" && (
        <p style={{ color: "var(--text-faint)", fontSize: 15, textAlign: "center", padding: "40px 0" }}>
          Cargando alumnos…
        </p>
      )}
      {roll === "ready" && q.trim() === "" && (
        <p style={{ color: "var(--text-faint)", fontSize: 15, textAlign: "center", padding: "40px 0" }}>
          Escribe el nombre del alumno para empezar.
        </p>
      )}
      {roll === "ready" && q.trim() !== "" && matches.length === 0 && (
        <p style={{ color: "var(--text-muted)", fontSize: 15, textAlign: "center", padding: "40px 0" }}>
          Ningún alumno con ese nombre.
        </p>
      )}

      <div className="fade-up" style={{ background: "var(--surface)", border: matches.length > 0 ? "1px solid var(--border)" : "none", borderRadius: "var(--r-xl)", overflow: "hidden" }}>
        {matches.map((s) => {
          const due = owed.get(s.id);
          return (
            <button
              key={s.id}
              onClick={() => setCollecting({ id: s.id, firstName: s.firstName, lastName: s.lastName, gradeGroup: s.gradeGroup })}
              style={{ display: "flex", width: "100%", alignItems: "center", gap: 14, padding: "14px 20px", background: "transparent", border: "none", borderTop: "1px solid var(--border)", cursor: "pointer", textAlign: "left" }}>
              <Avatar name={`${s.firstName} ${s.lastName}`} color="forest" size={42} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 15.5, fontWeight: 600, color: "var(--text)" }}>{s.firstName} {s.lastName}</div>
                <div style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{s.level} · {s.gradeGroup}</div>
              </div>
              <div style={{ textAlign: "right" }}>
                <div style={{ fontSize: 12.5, color: "var(--text-faint)" }}>Debe hoy</div>
                <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16, color: due !== undefined && due > 0 ? "var(--text)" : "var(--text-faint)" }}>
                  {due === undefined ? "…" : formatCents(due)}
                </div>
              </div>
            </button>
          );
        })}
      </div>

      {collecting !== null && (
        <CollectDrawer
          student={collecting}
          canGrantDiscounts={canGrantDiscounts}
          onClose={() => setCollecting(null)}
          onCollected={() => {
            // The figure this screen shows is now stale for that pupil.
            setOwed((prev) => { const next = new Map(prev); next.delete(collecting.id); return next; });
            say("Pago registrado");
          }}
        />
      )}
      {toast !== null && (
        <div role="status" style={{ position: "fixed", bottom: 28, left: "50%", transform: "translateX(-50%)", zIndex: 80, display: "flex", alignItems: "center", gap: 11, padding: "14px 22px", borderRadius: "var(--r-pill)", background: "var(--forest)", color: "#fff", fontWeight: 600, fontSize: 15, boxShadow: "var(--shadow-pop)" }}>
          <Icon name="check" size={20} stroke={2.6} />{toast}
        </div>
      )}
      {cashUp && <CashUpDrawer onClose={() => setCashUp(false)} />}
    </Page>
  );
}

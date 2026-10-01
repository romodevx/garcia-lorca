"use client";

import { useEffect, useMemo, useState, type CSSProperties, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import {
  correctDeposit,
  getAccount,
  listDepositCorrections,
  listDeposits,
  type Charge,
  type CorrectionResult,
  type Deposit,
  type DepositCorrection,
  type DepositSnapshot,
} from "../../finance/api";
import { centsToPesosInput, formatCents, pesosToCents } from "../../_lib/money";
import { localISODate } from "../../_lib/localDate";
import { ticketPath } from "../../receipts/api";
import { Btn } from "../Controls";
import { DrawerShell } from "../DrawerShell";
import { DateField, MoneyField, TextArea } from "../form";
import { useFocusFirstInvalid } from "../form/useFocusFirstInvalid";
import { TenderEditor } from "./TenderEditor";
import { tenderSummary } from "./tenders";
import { useTenders } from "./useTenders";

/** The contract's bound on a correction's reason. */
const MAX_REASON = 500;

export interface CorrectPaymentDrawerProps {
  depositId: string;
  /** The pupil whose payment it is: their receipts and charges are read. */
  studentId: string;
  /** The office's day (aaaa-mm-dd), the latest a payment can be dated; injected by tests. */
  today?: string;
  /** Closing — after a correction or not — is when the screen behind re-reads what it shows. */
  onClose: () => void;
}

/** Everything the form needs, read once when the drawer opens. */
interface Loaded {
  deposit: Deposit;
  charges: Charge[];
  /** The charges this receipt paid: what the correction preselects. */
  paid: string[];
  history: DepositCorrection[];
}

type Load = { state: "loading" } | { state: "failed"; message: string } | { state: "ready"; data: Loaded };

/**
 * Mint a handle on ONE correction attempt, reused across its retries: a lost
 * answer retried must not correct twice.
 */
function newAttemptKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

async function load(depositId: string, studentId: string): Promise<Loaded> {
  const [deposits, account, history] = await Promise.all([
    listDeposits(API_URL, studentId),
    getAccount(API_URL, studentId),
    listDepositCorrections(API_URL, depositId),
  ]);
  const deposit = deposits.find((d) => d.id === depositId);
  if (deposit === undefined) throw new Error("No se encontró ese pago.");
  const paid = [...new Set(account.payments.filter((p) => p.depositId === depositId).map((p) => p.chargeId))];
  // What the money may pay: what this receipt paid, and whatever is still owed.
  const charges = account.charges.filter((c) => c.status !== "cancelled" && (paid.includes(c.id) || c.remainingCents > 0));
  return { deposit, charges, paid, history };
}

/**
 * Correct a registered payment (T9).
 *
 * Nothing is edited in place: the original receipt is kept and marked
 * "Sustituido", and the server issues a new one with its own folio that names
 * it. The amount, day, methods and concepts are the ones the office may fix;
 * the reason is required. Five fields, one drawer — and the history of the
 * receipt underneath, closed until asked for.
 */
export function CorrectPaymentDrawer({ depositId, studentId, today, onClose }: CorrectPaymentDrawerProps): ReactElement {
  const [loaded, setLoaded] = useState<Load>({ state: "loading" });

  useEffect(() => {
    let live = true;
    load(depositId, studentId)
      .then((data) => { if (live) setLoaded({ state: "ready", data }); })
      .catch((cause: unknown) => {
        if (live) setLoaded({ state: "failed", message: cause instanceof Error ? cause.message : "No se pudo leer el pago." });
      });
    return () => { live = false; };
  }, [depositId, studentId]);

  if (loaded.state !== "ready") {
    return (
      <DrawerShell title="Corregir pago" icon="finance" onClose={onClose}>
        {loaded.state === "loading"
          ? <p style={{ color: "var(--text-muted)" }}>Cargando pago…</p>
          : <p role="alert" style={{ color: "var(--red)", margin: 0 }}>{loaded.message}</p>}
      </DrawerShell>
    );
  }
  return <CorrectionForm data={loaded.data} today={today ?? localISODate()} onClose={onClose} />;
}

interface FormErrors {
  amount?: string;
  paidAt?: string;
  reason?: string;
}

function CorrectionForm({ data, today, onClose }: { data: Loaded; today: string; onClose: () => void }): ReactElement {
  const { deposit } = data;
  const title = `Corregir pago ${deposit.folio ?? ""}`.trim();
  const [amount, setAmount] = useState(centsToPesosInput(deposit.amountCents));
  // The recorded day as stored (the school's day is the UTC day). Untouched, it
  // is not sent: the server keeps the original's instant, and a payment taken
  // late in the evening is not refused as "future" by the office's clock.
  const recordedDay = deposit.paidAt.slice(0, 10);
  const [paidAt, setPaidAt] = useState(recordedDay);
  const [chosen, setChosen] = useState<readonly string[]>(data.paid);
  const [reason, setReason] = useState("");
  const [errors, setErrors] = useState<FormErrors>({});
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [done, setDone] = useState<CorrectionResult | undefined>(undefined);
  const [attemptKey] = useState(newAttemptKey);
  const totalCents = pesosToCents(amount);
  const tenders = useTenders(totalCents, deposit.tenders);
  const focusFirstInvalid = useFocusFirstInvalid();

  const supersededBy = deposit.supersededByFolio ?? deposit.supersededBy;
  if (supersededBy !== undefined || deposit.voidedAt !== undefined) {
    return (
      <DrawerShell title={title} icon="finance" onClose={onClose}>
        <p role="status" style={{ margin: 0 }}>
          {supersededBy !== undefined
            ? `Este pago ya fue corregido por ${supersededBy}. Para cambiarlo otra vez, corrige ese comprobante.`
            : "Este pago está cancelado: ya no se puede corregir."}
        </p>
        <History history={data.history} charges={data.charges} />
      </DrawerShell>
    );
  }

  const amountError = (): string | undefined =>
    Number.isFinite(totalCents) && totalCents > 0 ? undefined : "Escribe el monto que la familia entregó.";
  const paidAtError = (): string | undefined => {
    if (paidAt === "") return "Elige el día del pago.";
    if (paidAt === recordedDay) return undefined;
    return paidAt > today ? "La fecha del pago no puede estar en el futuro." : undefined;
  };
  const reasonError = (): string | undefined => {
    const trimmed = reason.trim();
    if (trimmed === "") return "Escribe por qué se corrige el pago.";
    return [...trimmed].length > MAX_REASON ? "El motivo admite como máximo 500 caracteres." : undefined;
  };

  const save = async (): Promise<void> => {
    if (saving) return;
    const found: FormErrors = { amount: amountError(), paidAt: paidAtError(), reason: reasonError() };
    const tendersOk = tenders.validate();
    setErrors(found);
    if (found.amount !== undefined || found.paidAt !== undefined || found.reason !== undefined || !tendersOk) {
      focusFirstInvalid();
      return;
    }
    setSaving(true);
    setProblem(undefined);
    try {
      const result = await correctDeposit(API_URL, deposit.id, {
        amountCents: totalCents,
        paidAt: paidAt === recordedDay ? "" : paidAt,
        tenders: tenders.toTenders(),
        chargeIds: [...chosen],
        reason: reason.trim(),
        idempotencyKey: attemptKey,
      });
      setDone(result);
    } catch (cause) {
      setProblem(cause instanceof Error ? cause.message : "No se pudo corregir el pago.");
    } finally {
      setSaving(false);
    }
  };

  // Saved: the drawer turns into the new receipt to print.
  if (done !== undefined) {
    return (
      <DrawerShell
        title="Pago corregido"
        subtitle={`${done.deposit.folio ?? ""} sustituye a ${done.original.folio ?? ""}`}
        icon="finance"
        onClose={onClose}
        footer={<div style={{ display: "flex", justifyContent: "flex-end" }}><Btn onClick={onClose}>Listo</Btn></div>}
      >
        <div role="status" style={{ display: "grid", gap: 12 }}>
          <p style={{ margin: 0, fontSize: 16 }}>
            {`Se emitió el comprobante ${done.deposit.folio ?? ""} por ${formatCents(done.deposit.amountCents)}. El ${done.original.folio ?? "anterior"} queda como sustituido. La familia recibirá un aviso con el monto correcto.`}
          </p>
          <div>
            <Btn kind="ghost" icon="documents" onClick={() => window.open(ticketPath("payment", done.deposit.id), "_blank", "noopener")}>
              Reimprimir comprobante
            </Btn>
          </div>
        </div>
      </DrawerShell>
    );
  }

  const footer = (
    <div style={{ display: "flex", gap: 12, alignItems: "center", justifyContent: "flex-end", flexWrap: "wrap" }}>
      <button type="button" onClick={onClose} style={textButton}>Cancelar</button>
      <Btn type="submit">{saving ? "Guardando…" : "Guardar corrección"}</Btn>
    </div>
  );

  return (
    <DrawerShell
      title={title}
      subtitle={`Registrado por ${formatCents(deposit.amountCents)} · ${tenderSummary(deposit.tenders)}`}
      icon="finance"
      onClose={onClose}
      onSubmit={() => void save()}
      dismissible={false}
      footer={footer}
    >
      <div style={{ display: "grid", gap: 18 }}>
        <p style={{ margin: 0, fontSize: 14, color: "var(--text-muted)" }}>
          El comprobante original no se borra: queda como sustituido y se emite uno nuevo que lo corrige.
        </p>
        <MoneyField
          label="Monto recibido"
          required
          value={amount}
          onChange={setAmount}
          error={errors.amount}
          onClearError={() => setErrors((e) => ({ ...e, amount: undefined }))}
          onBlur={() => setErrors((e) => ({ ...e, amount: amountError() }))}
        />
        <DateField
          label="Fecha de pago"
          required
          value={paidAt}
          max={recordedDay > today ? recordedDay : today}
          onChange={setPaidAt}
          error={errors.paidAt}
          onClearError={() => setErrors((e) => ({ ...e, paidAt: undefined }))}
          onBlur={() => setErrors((e) => ({ ...e, paidAt: paidAtError() }))}
        />
        <TenderEditor tenders={tenders} />
        <Concepts charges={data.charges} paid={data.paid} chosen={chosen} onChange={setChosen} />
        <TextArea
          label="Motivo de la corrección"
          required
          rows={3}
          value={reason}
          onChange={setReason}
          error={errors.reason}
          help="Queda en el historial del pago, con tu nombre y la fecha."
          onClearError={() => setErrors((e) => ({ ...e, reason: undefined }))}
          onBlur={() => setErrors((e) => ({ ...e, reason: reasonError() }))}
        />
        {problem !== undefined && <p role="alert" style={{ color: "var(--red)", margin: 0 }}>{problem}</p>}
        <History history={data.history} charges={data.charges} />
      </div>
    </DrawerShell>
  );
}

const textButton: CSSProperties = {
  minHeight: 44, padding: "0 14px", fontSize: 15.5, fontWeight: 600, color: "var(--text-muted)", background: "none", border: "none", cursor: "pointer",
};

/**
 * What the money may pay. Preselected: what the receipt paid. None chosen is
 * the ordinary rule — oldest debt first — and the hint says so.
 */
function Concepts({ charges, paid, chosen, onChange }: { charges: Charge[]; paid: readonly string[]; chosen: readonly string[]; onChange: (ids: readonly string[]) => void }): ReactElement {
  const toggle = (id: string): void => onChange(chosen.includes(id) ? chosen.filter((c) => c !== id) : [...chosen, id]);
  return (
    <fieldset style={{ border: "none", padding: 0, margin: 0, display: "grid", gap: 8 }}>
      <legend style={{ fontSize: 14.5, fontWeight: 600, color: "var(--text)", marginBottom: 6 }}>Conceptos que paga</legend>
      <span style={{ fontSize: 13, color: "var(--text-muted)" }}>Si no eliges ninguno, se aplica al adeudo más antiguo.</span>
      {charges.map((c) => (
        <label key={c.id} style={{ display: "flex", alignItems: "center", gap: 10, minHeight: 44, fontSize: 15 }}>
          <input type="checkbox" checked={chosen.includes(c.id)} onChange={() => toggle(c.id)} style={{ width: 20, height: 20 }} />
          <span style={{ flex: 1 }}>{c.description}</span>
          <span style={{ fontSize: 13, color: "var(--text-muted)" }}>
            {paid.includes(c.id) ? "pagado con este recibo" : `debe ${formatCents(c.remainingCents)}`}
          </span>
        </label>
      ))}
    </fieldset>
  );
}

/** The receipt's history: each correction, before → after, who, when and why. */
function History({ history, charges }: { history: DepositCorrection[]; charges: Charge[] }): ReactElement | null {
  const names = useMemo(() => new Map(charges.map((c) => [c.id, c.description])), [charges]);
  if (history.length === 0) return null;
  return (
    <details style={{ borderTop: "1px solid var(--border)", paddingTop: 12 }}>
      <summary style={{ cursor: "pointer", fontWeight: 600, minHeight: 44, display: "flex", alignItems: "center" }}>
        {`Historial de cambios (${history.length})`}
      </summary>
      <ol style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 14 }}>
        {[...history].reverse().map((c) => (
          <li key={c.id} style={{ display: "grid", gap: 4, fontSize: 14 }}>
            <span style={{ color: "var(--text-muted)" }}>
              {[formatInstant(c.correctedAt), c.correctedByName].filter((p) => p !== undefined && p !== "").join(" · ")}
            </span>
            <span><strong>Motivo:</strong> {c.reason}</span>
            <span><strong>Antes:</strong> {snapshotText(c.before, names)}</span>
            <span><strong>Después:</strong> {snapshotText(c.after, names)}</span>
          </li>
        ))}
      </ol>
    </details>
  );
}

/** "P-2026-000001 · $500.00 · 9 sep 2026 · Efectivo · Colegiatura septiembre". */
function snapshotText(s: DepositSnapshot, names: Map<string, string>): string {
  const concepts = s.lines.map((l) => l.description ?? names.get(l.chargeId) ?? "").filter((d) => d !== "").join(", ");
  return [s.folio, formatCents(s.amountCents), s.paidAt === undefined ? "" : formatDay(s.paidAt), tenderSummary(s.tenders), concepts]
    .filter((p) => p !== "")
    .join(" · ");
}

/** A calendar day as written (UTC midnight is the school's day). */
function formatDay(iso: string): string {
  const at = new Date(iso);
  return Number.isNaN(at.getTime()) ? iso : at.toLocaleDateString("es-MX", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
}

/** The moment a correction was saved, on the office's clock. */
function formatInstant(iso: string): string {
  const at = new Date(iso);
  return Number.isNaN(at.getTime()) ? "" : at.toLocaleString("es-MX", { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

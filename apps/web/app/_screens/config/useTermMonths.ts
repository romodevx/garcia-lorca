import { useCallback, useEffect, useRef, useState } from "react";

import { ConfigError, type Term } from "../../config/api";
import { assignMonth, savePlan, type MonthAssignment, type TermMonthsSave } from "../../config/termMonths";
import { monthChips } from "../../grades/monthly";

export type SaveTermMonths = (termId: string, months: readonly string[]) => Promise<Term>;

export interface TermMonthsState {
  assignment: MonthAssignment;
  error: string | undefined;
  /** One tap on (term, month): shown at once, saved behind, undone alone if refused. */
  tap: (termId: string, month: string) => void;
}

interface Tap {
  termId: string;
  month: string;
}

const FALLBACK_REASON = "No se pudo guardar. Quedó como estaba.";
const NOT_RESTORED = "y no se pudo devolver a su periodo; recarga la página para ver cómo quedó.";

/** The server's cap on a term's months (TERM_TOO_LONG), checked here before any PUT. */
const MAX_MONTHS_PER_TERM = 6;
const TERM_CLOSED = "TERM_CLOSED";
const TERM_TOO_LONG = "TERM_TOO_LONG";

/** Why a month-write was refused, in words, for the codes the server may omit words for. */
const REASON_BY_CODE: Readonly<Record<string, string>> = {
  TERM_CLOSED: "El periodo está cerrado; reábrelo para cambiar sus meses.",
  TERM_TOO_LONG: "Un periodo puede tener hasta 6 meses.",
  MONTH_IN_OTHER_TERM: "Ese mes ya es de otro periodo.",
  MONTH_INVALID: "Ese mes no es válido para este ciclo.",
};

function assignmentOf(terms: readonly Term[]): MonthAssignment {
  return Object.fromEntries(terms.map((t) => [t.id, [...(t.months ?? [])].sort()]));
}

/** A refusal decided here, from what is already known, before any PUT. */
class LocalRefusal extends ConfigError {
  constructor(code: string) {
    super(0, "", code);
  }
}

/** A step of a move that failed after an earlier step was saved, and whether that step was undone. */
class PartialMove extends Error {
  constructor(readonly refusal: unknown, readonly restored: boolean) {
    super("partial move");
  }
}

function refusalReason(err: unknown): string {
  if (err instanceof PartialMove) return refusalReason(err.refusal);
  if (!(err instanceof ConfigError)) return FALLBACK_REASON;
  if (err.detail !== "") return err.detail;
  return (err.code !== undefined ? REASON_BY_CODE[err.code] : undefined) ?? FALLBACK_REASON;
}

/** "No se guardó noviembre de 2026: Un periodo puede tener hasta 6 meses." */
function refusalMessage(month: string, err: unknown): string {
  const name = monthChips([month])[0]?.fullName ?? month;
  if (err instanceof PartialMove && !err.restored) return `No se guardó ${name} ${NOT_RESTORED}`;
  return `No se guardó ${name}: ${refusalReason(err)}`;
}

/** The first term the plan would grow into a refusal the server is known to give. */
function knownRefusal(plan: readonly TermMonthsSave[], before: MonthAssignment, closed: ReadonlySet<string>): LocalRefusal | undefined {
  for (const step of plan) {
    const grows = step.months.some((m) => !(before[step.termId] ?? []).includes(m));
    if (!grows) continue;
    if (closed.has(step.termId)) return new LocalRefusal(TERM_CLOSED);
    if (step.months.length > MAX_MONTHS_PER_TERM) return new LocalRefusal(TERM_TOO_LONG);
  }
  return undefined;
}

/** Keeps what is known for the ids still present, and seeds only the ids that are new. */
function reseed(current: MonthAssignment, terms: readonly Term[]): MonthAssignment {
  const fresh = assignmentOf(terms);
  return Object.fromEntries(Object.keys(fresh).map((id) => [id, current[id] ?? fresh[id] ?? []]));
}

/**
 * The level's month-to-term layout with optimistic taps. The server's
 * confirmed copy plus the taps still waiting: the screen shows the taps
 * replayed on the confirmed copy. Taps are saved one at a time, in order, so
 * fast taps never race; a refused tap is dropped alone and the taps behind it
 * are replayed on what the server did confirm.
 */
export function useTermMonths(terms: readonly Term[], save: SaveTermMonths): TermMonthsState {
  const [assignment, setAssignment] = useState<MonthAssignment>(() => assignmentOf(terms));
  const [error, setError] = useState<string | undefined>(undefined);
  const confirmed = useRef<MonthAssignment>(assignment);
  const pending = useRef<Tap[]>([]);
  const queue = useRef<Promise<void>>(Promise.resolve());
  // Terms the server refused as closed: a later move into one is refused here, before any PUT.
  const closedTerms = useRef<Set<string>>(new Set());

  const show = useCallback((): void => {
    setAssignment(pending.current.reduce((acc, t) => assignMonth(acc, t.termId, t.month), confirmed.current));
  }, []);

  // A term joined or left (one was just created): seed only the new ids, so a
  // parent list older than a save in flight never reverts months already saved.
  const termIds = terms.map((t) => t.id).join("|");
  useEffect(() => {
    confirmed.current = reseed(confirmed.current, terms);
    show();
  }, [termIds, show]);

  const putMonths = useCallback(async (termId: string, months: readonly string[]): Promise<void> => {
    const saved = await save(termId, months);
    confirmed.current = { ...confirmed.current, [termId]: [...(saved.months ?? months)].sort() };
  }, [save]);

  /** Puts the given terms back to `before`, newest first; says whether all of them went back. */
  const restore = useCallback(async (termIds: readonly string[], before: MonthAssignment): Promise<boolean> => {
    try {
      for (const id of [...termIds].reverse()) await putMonths(id, before[id] ?? []);
      return true;
    } catch {
      return false;
    }
  }, [putMonths]);

  /**
   * Runs the plan's PUTs in order. A step refused after earlier steps were
   * saved undoes those steps (a month freed from its term goes back), so a
   * half-done move never strands a month outside every term.
   */
  const applyPlan = useCallback(async (plan: readonly TermMonthsSave[]): Promise<void> => {
    const before = confirmed.current;
    const done: string[] = [];
    for (const step of plan) {
      try {
        await putMonths(step.termId, step.months);
        done.push(step.termId);
      } catch (err) {
        if (err instanceof ConfigError && err.code === TERM_CLOSED) closedTerms.current.add(step.termId);
        if (done.length === 0) throw err;
        throw new PartialMove(err, await restore(done, before));
      }
    }
  }, [putMonths, restore]);

  const saveNext = useCallback(async (): Promise<void> => {
    const next = pending.current[0];
    if (next === undefined) return;
    const plan = savePlan(confirmed.current, assignMonth(confirmed.current, next.termId, next.month));
    try {
      const refusal = knownRefusal(plan, confirmed.current, closedTerms.current);
      if (refusal !== undefined) throw refusal;
      await applyPlan(plan);
    } catch (err) {
      setError(refusalMessage(next.month, err));
    } finally {
      pending.current = pending.current.slice(1);
      show();
    }
  }, [applyPlan, show]);

  const tap = useCallback((termId: string, month: string): void => {
    pending.current = [...pending.current, { termId, month }];
    // A new tap is a new attempt; a refusal stays on screen until then, even if taps behind it succeed.
    setError(undefined);
    show();
    queue.current = queue.current.then(saveNext);
  }, [saveNext, show]);

  return { assignment, error, tap };
}

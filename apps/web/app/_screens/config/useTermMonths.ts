import { useCallback, useEffect, useRef, useState } from "react";

import { ConfigError, type Term } from "../../config/api";
import { assignMonth, savePlan, type MonthAssignment } from "../../config/termMonths";
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

function refusalReason(err: unknown): string {
  if (!(err instanceof ConfigError)) return FALLBACK_REASON;
  if (err.detail !== "") return err.detail;
  return (err.code !== undefined ? REASON_BY_CODE[err.code] : undefined) ?? FALLBACK_REASON;
}

/** "No se guardó noviembre de 2026: Un periodo puede tener hasta 6 meses." */
function refusalMessage(month: string, err: unknown): string {
  const name = monthChips([month])[0]?.fullName ?? month;
  return `No se guardó ${name}: ${refusalReason(err)}`;
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

  const saveNext = useCallback(async (): Promise<void> => {
    const next = pending.current[0];
    if (next === undefined) return;
    const target = assignMonth(confirmed.current, next.termId, next.month);
    try {
      for (const step of savePlan(confirmed.current, target)) {
        const saved = await save(step.termId, step.months);
        confirmed.current = { ...confirmed.current, [step.termId]: [...(saved.months ?? step.months)].sort() };
      }
    } catch (err) {
      setError(refusalMessage(next.month, err));
    } finally {
      pending.current = pending.current.slice(1);
      show();
    }
  }, [save, show]);

  const tap = useCallback((termId: string, month: string): void => {
    pending.current = [...pending.current, { termId, month }];
    // A new tap is a new attempt; a refusal stays on screen until then, even if taps behind it succeed.
    setError(undefined);
    show();
    queue.current = queue.current.then(saveNext);
  }, [saveNext, show]);

  return { assignment, error, tap };
}

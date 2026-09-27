import { useCallback, useEffect, useRef, useState } from "react";

import { ConfigError, type Term } from "../../config/api";
import { assignMonth, savePlan, type MonthAssignment } from "../../config/termMonths";

export type SaveTermMonths = (termId: string, months: readonly string[]) => Promise<Term>;

export interface TermMonthsState {
  assignment: MonthAssignment;
  error: string | undefined;
  /** One tap on (term, month): shown at once, saved behind, reverted if refused. */
  tap: (termId: string, month: string) => void;
}

const FALLBACK_ERROR = "No se pudo guardar el mes. Quedó como estaba.";

function assignmentOf(terms: readonly Term[]): MonthAssignment {
  return Object.fromEntries(terms.map((t) => [t.id, [...(t.months ?? [])].sort()]));
}

function refusalMessage(err: unknown): string {
  return err instanceof ConfigError && err.detail !== "" ? err.detail : FALLBACK_ERROR;
}

/**
 * The level's month-to-term layout with optimistic taps. Two copies: what the
 * server confirmed and what the director asked for. Saves run one after the
 * other, each bringing the confirmed copy up to the wanted one, so fast taps
 * never race; a refusal puts the screen back on the confirmed copy.
 */
export function useTermMonths(terms: readonly Term[], save: SaveTermMonths): TermMonthsState {
  const [assignment, setAssignment] = useState<MonthAssignment>(() => assignmentOf(terms));
  const [error, setError] = useState<string | undefined>(undefined);
  const confirmed = useRef<MonthAssignment>(assignment);
  const wanted = useRef<MonthAssignment>(assignment);
  const queue = useRef<Promise<void>>(Promise.resolve());

  // A new set of terms (one was just created) is a new truth; months saved here never change the ids,
  // so the effect is keyed on the ids alone and never clobbers taps still being saved.
  const termIds = terms.map((t) => t.id).join("|");
  useEffect(() => {
    const fresh = assignmentOf(terms);
    confirmed.current = fresh;
    wanted.current = fresh;
    setAssignment(fresh);
  }, [termIds]);

  const sync = useCallback(async (): Promise<void> => {
    try {
      for (const step of savePlan(confirmed.current, wanted.current)) {
        await save(step.termId, step.months);
        confirmed.current = { ...confirmed.current, [step.termId]: step.months };
      }
      setError(undefined);
    } catch (err) {
      wanted.current = confirmed.current;
      setAssignment(confirmed.current);
      setError(refusalMessage(err));
    }
  }, [save]);

  const tap = useCallback((termId: string, month: string): void => {
    wanted.current = assignMonth(wanted.current, termId, month);
    setAssignment(wanted.current);
    queue.current = queue.current.then(sync);
  }, [sync]);

  return { assignment, error, tap };
}

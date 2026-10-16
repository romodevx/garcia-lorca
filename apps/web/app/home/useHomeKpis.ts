import { useCallback, useEffect, useState } from "react";

import { API_URL } from "../api-url";
import { listCycles, type Cycle } from "../config/api";
import { listCharges } from "../finance/api";
import { isOwed, pendingCents, type OwedReading } from "../finance/pending";
import { listStudents } from "../students/api";

/**
 * The home's KPI reads (B4–B6), with their outcome kept instead of swallowed
 * (PLAN-redesign-home §3.4, review D2): a zero shown while loading, or after a
 * 500, is a claim the school would act on. The classic home reads these as
 * plain numbers (0 until ready), exactly as before; the refreshed home shows
 * every state.
 */
export type Reading<T> = { status: "idle" } | { status: "loading" } | { status: "error" } | { status: "ready"; value: T };

export interface Ledger {
  pendingCents: number;
  collectedCents: number;
  /** Charges with something still owed. */
  withBalance: number;
  /** Every charge of the cycle, paid or not: zero means none was ever issued. */
  charges: number;
}

/** `noCycle`: the school has no cycle at all, so there is no ledger to read. */
export type LedgerReading = Reading<Ledger> | { status: "noCycle" };

export interface CycleInfo {
  name: string;
  /** False when no cycle is active and the first one stood in for it. */
  active: boolean;
}

/** Which reads the caller may make (B6: a maestro makes none). */
export interface KpiAccess {
  students: boolean;
  cycles: boolean;
  /** Implies `cycles`: the ledger is read per cycle. */
  ledger: boolean;
}

export interface HomeKpis {
  students: Reading<number>;
  cycle: CycleInfo | undefined;
  ledger: LedgerReading;
  retryStudents: () => void;
  retryLedger: () => void;
}

/** A result, tagged with the attempt that produced it: a retry is loading until its own answer lands. */
type Settled<T> = { attempt: number; value: T };

const IDLE = { status: "idle" } as const;
const LOADING = { status: "loading" } as const;
const ERROR = { status: "error" } as const;

/** "Registrados": the active admission pipeline — rejected and withdrawn are not current students. */
function countCurrent(students: readonly { admissionStatus: string }[]): number {
  return students.filter((s) => s.admissionStatus !== "rejected" && s.admissionStatus !== "withdrawn").length;
}

function sumLedger(charges: readonly (OwedReading & { paidCents: number })[]): Ledger {
  return {
    collectedCents: charges.reduce((a, c) => a + c.paidCents, 0),
    // Finanzas' rule, shared: a cancelled charge is never owed (T5).
    pendingCents: pendingCents(charges),
    withBalance: charges.filter(isOwed).length,
    charges: charges.length,
  };
}

/** The active cycle, or — as the home always did — the first one standing in for it. */
function currentCycle(cycles: readonly Cycle[]): Cycle | undefined {
  return cycles.find((c) => c.status === "active") ?? cycles[0];
}

function useStudentsCount(allowed: boolean): [Reading<number>, () => void] {
  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState<Settled<Reading<number>>>();
  useEffect(() => {
    if (!allowed) return;
    let live = true;
    const settle = (value: Reading<number>): void => {
      if (live) setResult({ attempt, value });
    };
    listStudents(API_URL).then((ss) => settle({ status: "ready", value: countCurrent(ss) }), () => settle(ERROR));
    return () => { live = false; };
  }, [allowed, attempt]);
  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  if (!allowed) return [IDLE, retry];
  return [result?.attempt === attempt ? result.value : LOADING, retry];
}

function useCycleLedger(access: KpiAccess): [CycleInfo | undefined, LedgerReading, () => void] {
  const [attempt, setAttempt] = useState(0);
  const [cycle, setCycle] = useState<CycleInfo>();
  const [ledger, setLedger] = useState<Settled<LedgerReading>>();
  const { cycles: readsCycles, ledger: readsLedger } = access;
  useEffect(() => {
    if (!readsCycles) return;
    let live = true;
    const settle = (value: LedgerReading): void => {
      if (live) setLedger({ attempt, value });
    };
    listCycles(API_URL).then(
      (cs) => {
        // Unmounted (or superseded) while the cycles were in flight: no ledger read.
        if (!live) return;
        const cyc = currentCycle(cs);
        if (cyc === undefined) return settle({ status: "noCycle" });
        setCycle({ name: cyc.name, active: cyc.status === "active" });
        if (!readsLedger) return;
        listCharges(API_URL, { cycleId: cyc.id }).then((chs) => settle({ status: "ready", value: sumLedger(chs) }), () => settle(ERROR));
      },
      () => settle(ERROR),
    );
    return () => { live = false; };
  }, [readsCycles, readsLedger, attempt]);
  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  if (!readsLedger) return [cycle, IDLE, retry];
  return [cycle, ledger?.attempt === attempt ? ledger.value : LOADING, retry];
}

export function useHomeKpis(access: KpiAccess): HomeKpis {
  const [students, retryStudents] = useStudentsCount(access.students);
  const [cycle, ledger, retryLedger] = useCycleLedger(access);
  return { students, cycle, ledger, retryStudents, retryLedger };
}

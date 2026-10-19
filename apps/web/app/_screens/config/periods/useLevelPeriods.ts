"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { API_URL } from "../../../api-url";
import {
  ConfigError,
  copyEvaluationPeriods,
  listCycles,
  listEvaluationPeriods,
  listGradeLevels,
  listLevels,
  saveGradeEvaluationPeriods,
  type Cycle,
  type EvaluationPeriod,
  type GradeLevel,
  type Level,
} from "../../../config/api";
import {
  draftsOf,
  nextDraft,
  periodProblems,
  rowProblems,
  toInputs,
  type PeriodDraft,
  type PeriodField,
  type PeriodProblem,
} from "../../../config/evaluationPeriods";

/** How long a plain notice stays. "Deshacer" has no clock: the person closes it (ISSUE-CC-105). */
export const NOTICE_MS = 8_000;

const FIELD_ORDER: readonly PeriodField[] = ["name", "startsOn", "endsOn"];
const SAVE_FAILED = "No se pudieron guardar los periodos. Intenta de nuevo.";
const COPY_FAILED = "No se pudieron copiar los periodos. Intenta de nuevo.";
const RESTORE_FAILED = "No se pudieron restaurar los periodos de este grado.";
const SOURCE_EMPTY = "Este grado aún no tiene periodos guardados para copiar.";

type ByGrade<T> = Readonly<Record<string, T>>;

export type PeriodsLoad = { kind: "loading" } | { kind: "failed" } | { kind: "ready" };

/** A copy that can still be taken back: each target's periods before it. */
export interface PendingUndo {
  message: string;
  previous: ByGrade<EvaluationPeriod[]>;
}

/** What copying did: whether it landed, and the field to fix when it could not. */
export interface CopyOutcome {
  copied: boolean;
  /** The first wrong field of the source's unsaved values, in reading order. */
  focus?: PeriodProblem;
}

/** One grade's set written to the server: what it holds now, or why it was refused. */
type Persisted =
  | { kind: "saved"; periods: EvaluationPeriod[] }
  | { kind: "invalid"; first: PeriodProblem | undefined }
  | { kind: "failed" }
  | { kind: "stale" };

export interface LevelPeriods {
  load: PeriodsLoad;
  level: Level | undefined;
  cycles: Cycle[];
  cycle: Cycle | undefined;
  grades: GradeLevel[];
  drafts: ByGrade<PeriodDraft[]>;
  problems: ByGrade<PeriodProblem[]>;
  errors: ByGrade<string | undefined>;
  busy: ByGrade<boolean>;
  notice: string | undefined;
  undo: PendingUndo | undefined;
  selectCycle: (id: string) => void;
  /** Reads everything again after a failed load. */
  retry: () => void;
  edit: (gradeId: string, key: string, field: PeriodField, value: string) => void;
  blur: (gradeId: string, key: string, field: PeriodField) => void;
  add: (gradeId: string) => string;
  remove: (gradeId: string, key: string) => void;
  save: (gradeId: string) => Promise<PeriodProblem | undefined>;
  /** Copies what the source shows on screen: unsaved values are saved first (C2-001). */
  copy: (fromGradeId: string, toGradeIds: readonly string[]) => Promise<CopyOutcome>;
  takeBack: () => Promise<void>;
  /** The person closed the Deshacer offer: the copy stays. */
  dismissUndo: () => void;
}

function groupByGrade(periods: readonly EvaluationPeriod[]): Record<string, EvaluationPeriod[]> {
  const out: Record<string, EvaluationPeriod[]> = {};
  for (const p of periods) (out[p.gradeLevelId] ??= []).push(p);
  return out;
}

function sameSet(a: readonly PeriodDraft[], b: readonly PeriodDraft[]): boolean {
  return JSON.stringify(toInputs(a)) === JSON.stringify(toInputs(b));
}

/** The first problem in reading order: row by row, then name → start → end. */
function firstInOrder(problems: readonly PeriodProblem[], rows: readonly PeriodDraft[]): PeriodProblem | undefined {
  const rowIndex = (key: string): number => rows.findIndex((r) => r.key === key);
  return [...problems].sort(
    (a, b) => rowIndex(a.key) - rowIndex(b.key) || FIELD_ORDER.indexOf(a.field) - FIELD_ORDER.indexOf(b.field),
  )[0];
}

/**
 * The state of the "periodos por grado" page: the level's grades, one cycle at
 * a time, each grade's periods as editable rows. Validation follows the house
 * rule — a field is checked when it is left, everything on save, and an error
 * clears the moment the value is right again. The server re-checks all of it.
 */
export function useLevelPeriods(levelId: string): LevelPeriods {
  const [load, setLoad] = useState<PeriodsLoad>({ kind: "loading" });
  const [level, setLevel] = useState<Level | undefined>(undefined);
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [cycleId, setCycleId] = useState<string | undefined>(undefined);
  const [grades, setGrades] = useState<GradeLevel[]>([]);
  const [saved, setSaved] = useState<ByGrade<EvaluationPeriod[]>>({});
  const [drafts, setDrafts] = useState<ByGrade<PeriodDraft[]>>({});
  const [problems, setProblems] = useState<ByGrade<PeriodProblem[]>>({});
  const [errors, setErrors] = useState<ByGrade<string | undefined>>({});
  const [busy, setBusy] = useState<ByGrade<boolean>>({});
  const [notice, setNotice] = useState<string | undefined>(undefined);
  const [undo, setUndo] = useState<PendingUndo | undefined>(undefined);
  const newKeys = useRef(0);
  const [attempt, setAttempt] = useState(0);
  // The cycle on screen right now: an answer for another cycle is dropped.
  const shownCycle = useRef<string | undefined>(undefined);
  shownCycle.current = cycleId;

  const cycle = useMemo(() => cycles.find((c) => c.id === cycleId), [cycles, cycleId]);

  // The level, its grades and the cycles; the active cycle comes preselected.
  useEffect(() => {
    let live = true;
    Promise.all([listCycles(API_URL), listLevels(API_URL), listGradeLevels(API_URL, levelId)])
      .then(([cs, ls, gs]) => {
        if (!live) return;
        setCycles(cs);
        setLevel(ls.find((l) => l.id === levelId));
        setGrades([...gs].filter((g) => g.levelId === levelId).sort((a, b) => a.ordinal - b.ordinal));
        // A retry keeps the cycle the person had chosen.
        setCycleId((prev) => prev ?? (cs.find((c) => c.status === "active") ?? cs[cs.length - 1])?.id);
        if (cs.length === 0) setLoad({ kind: "ready" });
      })
      .catch(() => live && setLoad({ kind: "failed" }));
    return () => {
      live = false;
    };
    // attempt: Reintentar after a failed first read.
  }, [levelId, attempt]);

  const adopt = useCallback((byGrade: ByGrade<EvaluationPeriod[]>): void => {
    setSaved((prev) => ({ ...prev, ...byGrade }));
    setDrafts((prev) => {
      const next = { ...prev };
      for (const [g, ps] of Object.entries(byGrade)) next[g] = draftsOf(ps);
      return next;
    });
    setProblems((prev) => {
      const next = { ...prev };
      for (const g of Object.keys(byGrade)) next[g] = [];
      return next;
    });
    setErrors((prev) => {
      const next = { ...prev };
      for (const g of Object.keys(byGrade)) next[g] = undefined;
      return next;
    });
  }, []);

  // The cycle's periods of the whole level in one read; a stale answer (the
  // person already picked another cycle) is dropped.
  useEffect(() => {
    if (cycleId === undefined) return;
    let live = true;
    setLoad({ kind: "loading" });
    listEvaluationPeriods(API_URL, { cycleId, levelId })
      .then((ps) => {
        if (!live) return;
        const byGrade = groupByGrade(ps);
        setSaved({});
        setDrafts({});
        adopt(Object.fromEntries(grades.map((g) => [g.id, byGrade[g.id] ?? []])));
        setLoad({ kind: "ready" });
      })
      .catch(() => live && setLoad({ kind: "failed" }));
    return () => {
      live = false;
    };
  }, [cycleId, levelId, grades, adopt, attempt]);

  useEffect(() => {
    if (notice === undefined) return;
    const timer = setTimeout(() => setNotice(undefined), NOTICE_MS);
    return () => clearTimeout(timer);
  }, [notice]);

  const setGradeProblems = (gradeId: string, fn: (prev: PeriodProblem[]) => PeriodProblem[]): void => {
    setProblems((prev) => ({ ...prev, [gradeId]: fn(prev[gradeId] ?? []) }));
  };

  const edit = useCallback(
    (gradeId: string, key: string, field: PeriodField, value: string): void => {
      const rows = (drafts[gradeId] ?? []).map((r) => (r.key === key ? { ...r, [field]: value } : r));
      setDrafts((prev) => ({ ...prev, [gradeId]: rows }));
      setErrors((prev) => ({ ...prev, [gradeId]: undefined }));
      if (cycle === undefined) return;
      // An error leaves the moment it is no longer true; a new one waits for the blur.
      const still = periodProblems(rows, cycle);
      setGradeProblems(gradeId, (prev) =>
        prev.filter((p) => still.some((s) => s.key === p.key && s.field === p.field && s.message === p.message)),
      );
    },
    [drafts, cycle],
  );

  const blur = useCallback(
    (gradeId: string, key: string, field: PeriodField): void => {
      const row = (drafts[gradeId] ?? []).find((r) => r.key === key);
      if (row === undefined || cycle === undefined) return;
      const found = rowProblems(row, cycle).filter((p) => p.field === field);
      setGradeProblems(gradeId, (prev) => [...prev.filter((p) => !(p.key === key && p.field === field)), ...found]);
    },
    [drafts, cycle],
  );

  const add = useCallback(
    (gradeId: string): string => {
      newKeys.current += 1;
      const key = `new-${newKeys.current}`;
      const span = cycle ?? { startsOn: "", endsOn: "" };
      setDrafts((prev) => ({ ...prev, [gradeId]: [...(prev[gradeId] ?? []), nextDraft(prev[gradeId] ?? [], span, key)] }));
      return key;
    },
    [cycle],
  );

  const remove = useCallback((gradeId: string, key: string): void => {
    setDrafts((prev) => ({ ...prev, [gradeId]: (prev[gradeId] ?? []).filter((r) => r.key !== key) }));
    setGradeProblems(gradeId, (prev) => prev.filter((p) => p.key !== key));
  }, []);

  // Validates and writes one grade's rows; every answer for an old cycle is dropped.
  const persist = useCallback(
    async (gradeId: string, current: Cycle): Promise<Persisted> => {
      const rows = drafts[gradeId] ?? [];
      const found = periodProblems(rows, current);
      setProblems((prev) => ({ ...prev, [gradeId]: found }));
      if (found.length > 0) return { kind: "invalid", first: firstInOrder(found, rows) };
      setBusy((prev) => ({ ...prev, [gradeId]: true }));
      try {
        const result = await saveGradeEvaluationPeriods(API_URL, gradeId, current.id, toInputs(rows));
        if (shownCycle.current !== current.id) return { kind: "stale" };
        adopt({ [gradeId]: result });
        return { kind: "saved", periods: result };
      } catch (err) {
        if (shownCycle.current !== current.id) return { kind: "stale" };
        const detail = err instanceof ConfigError && err.detail !== "" ? err.detail : SAVE_FAILED;
        setErrors((prev) => ({ ...prev, [gradeId]: detail }));
        return { kind: "failed" };
      } finally {
        setBusy((prev) => ({ ...prev, [gradeId]: false }));
      }
    },
    [drafts, adopt],
  );

  const save = useCallback(
    async (gradeId: string): Promise<PeriodProblem | undefined> => {
      if (cycle === undefined || busy[gradeId] === true) return undefined;
      const outcome = await persist(gradeId, cycle);
      if (outcome.kind === "invalid") return outcome.first;
      if (outcome.kind === "saved") {
        const grade = grades.find((g) => g.id === gradeId);
        setNotice(`Periodos de ${grade?.name ?? "el grado"} guardados.`);
      }
      return undefined;
    },
    [cycle, busy, grades, persist],
  );

  // The source as the person sees it: saved first when it has unsaved values.
  const sourceOnScreen = useCallback(
    async (gradeId: string, current: Cycle): Promise<Persisted> => {
      const stored = saved[gradeId] ?? [];
      if (sameSet(drafts[gradeId] ?? [], draftsOf(stored))) return { kind: "saved", periods: stored };
      return persist(gradeId, current);
    },
    [saved, drafts, persist],
  );

  const copy = useCallback(
    async (fromGradeId: string, toGradeIds: readonly string[]): Promise<CopyOutcome> => {
      if (cycle === undefined || toGradeIds.length === 0 || busy[fromGradeId] === true) return { copied: false };
      const fail = (message: string): CopyOutcome => {
        setErrors((prev) => ({ ...prev, [fromGradeId]: message }));
        return { copied: false };
      };
      const source = await sourceOnScreen(fromGradeId, cycle);
      if (source.kind === "invalid") return { copied: false, focus: source.first };
      if (source.kind !== "saved") return { copied: false };
      if (source.periods.length === 0) return fail(SOURCE_EMPTY);
      const previous = Object.fromEntries(toGradeIds.map((g) => [g, saved[g] ?? []]));
      try {
        const result = groupByGrade(await copyEvaluationPeriods(API_URL, { cycleId: cycle.id, fromGradeLevelId: fromGradeId, toGradeLevelIds: [...toGradeIds] }));
        if (shownCycle.current !== cycle.id) return { copied: true };
        adopt(Object.fromEntries(toGradeIds.map((g) => [g, result[g] ?? []])));
        const n = toGradeIds.length;
        setNotice(undefined);
        setUndo({ message: n === 1 ? "Periodos copiados a 1 grado." : `Periodos copiados a ${n} grados.`, previous });
        return { copied: true };
      } catch (err) {
        return fail(err instanceof ConfigError && err.detail !== "" ? err.detail : COPY_FAILED);
      }
    },
    [cycle, busy, saved, sourceOnScreen, adopt],
  );

  const takeBack = useCallback(async (): Promise<void> => {
    if (undo === undefined || cycle === undefined) return;
    setUndo(undefined);
    for (const [gradeId, periods] of Object.entries(undo.previous)) {
      try {
        const restored = await saveGradeEvaluationPeriods(API_URL, gradeId, cycle.id, toInputs(draftsOf(periods)));
        if (shownCycle.current === cycle.id) adopt({ [gradeId]: restored });
      } catch (err) {
        // PERIOD_HAS_GRADES names the period that cannot go: say it, not a generic failure.
        const detail = err instanceof ConfigError && err.detail !== "" ? err.detail : RESTORE_FAILED;
        setErrors((prev) => ({ ...prev, [gradeId]: detail }));
      }
    }
  }, [undo, cycle, adopt]);

  const retry = useCallback((): void => setAttempt((n) => n + 1), []);

  const selectCycle = useCallback((id: string): void => {
    setUndo(undefined);
    setCycleId(id);
  }, []);

  const dismissUndo = useCallback((): void => setUndo(undefined), []);

  return {
    load, level, cycles, cycle, grades, drafts, problems, errors, busy, notice, undo,
    selectCycle, retry, edit, blur, add, remove, save, copy, takeBack, dismissUndo,
  };
}

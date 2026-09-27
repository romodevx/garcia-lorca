import { useCallback, useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { createTerm, listTerms, setTermMonths, type CreateTermInput, type Term } from "../../config/api";
import type { SaveTermMonths } from "./useTermMonths";

export type LevelTermsLoad = { kind: "idle" } | { kind: "loading" } | { kind: "ready"; terms: Term[] } | { kind: "failed" };

export interface LevelTerms {
  load: LevelTermsLoad;
  create: (input: CreateTermInput) => Promise<void>;
  setMonths: SaveTermMonths;
}

/** What was loaded, and for which (cycle, level): a list never outlives the selection it belongs to. */
interface Loaded {
  key: string;
  load: LevelTermsLoad;
}

const keyOf = (cycleId: string, levelId: string): string => `${cycleId}:${levelId}`;

/**
 * The terms of one (cycle, level). While another selection's list is still on
 * its way the answer is "loading" — never the previous level's terms — and a
 * failed read says so instead of looking like a level with no terms. Every
 * saved term (created, or with new months) is merged into the list by id.
 */
export function useLevelTerms(cycleId: string | undefined, levelId: string | undefined): LevelTerms {
  const key = cycleId !== undefined && levelId !== undefined ? keyOf(cycleId, levelId) : undefined;
  const [loaded, setLoaded] = useState<Loaded | undefined>(undefined);

  useEffect(() => {
    if (cycleId === undefined || levelId === undefined) return;
    const current = keyOf(cycleId, levelId);
    let active = true;
    listTerms(API_URL, cycleId, levelId)
      .then((terms) => active && setLoaded({ key: current, load: { kind: "ready", terms } }))
      .catch(() => active && setLoaded({ key: current, load: { kind: "failed" } }));
    return () => {
      active = false;
    };
  }, [cycleId, levelId]);

  const merge = useCallback((term: Term): void => {
    setLoaded((prev) => {
      if (prev?.key !== keyOf(term.cycleId, term.levelId) || prev.load.kind !== "ready") return prev;
      const terms = prev.load.terms;
      const next = terms.some((t) => t.id === term.id) ? terms.map((t) => (t.id === term.id ? { ...t, ...term } : t)) : [...terms, term];
      return { key: prev.key, load: { kind: "ready", terms: next } };
    });
  }, []);

  const create = useCallback(async (input: CreateTermInput): Promise<void> => merge(await createTerm(API_URL, input)), [merge]);

  const setMonths = useCallback<SaveTermMonths>(async (termId, months) => {
    const saved = await setTermMonths(API_URL, termId, months);
    merge(saved);
    return saved;
  }, [merge]);

  let load: LevelTermsLoad = { kind: "idle" };
  if (key !== undefined) load = loaded?.key === key ? loaded.load : { kind: "loading" };
  return { load, create, setMonths };
}

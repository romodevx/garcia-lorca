"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { API_URL } from "../../api-url";
import { createFamily, dismissFamilySuggestion, familyRefusal, listFamilySuggestions, type FamilySuggestion } from "../../families/api";

export interface FamilySuggestions {
  suggestions: readonly FamilySuggestion[];
  failed: boolean;
  /** Signatures with a write in flight. */
  busy: ReadonlySet<string>;
  /** Why a suggestion's last write was refused, by signature. */
  errors: Readonly<Record<string, string>>;
  /** Builds the family as suggested; resolves true when it was created. */
  accept: (s: FamilySuggestion) => Promise<boolean>;
  dismiss: (s: FamilySuggestion) => Promise<boolean>;
  /** Builds every "muy probable" one after the other; resolves how many were built. */
  acceptAllHigh: () => Promise<number>;
  /** A suggestion was taken care of elsewhere ("Ajustar"): it leaves the list. */
  drop: (signature: string) => void;
  /** A family was built: the others may have changed (a pupil is no longer free). */
  reload: () => void;
  /** The bulk acceptance is running: its button says so and ignores presses. */
  bulkRunning: boolean;
}

/**
 * The suggestions and the writes that answer them. Bulk acceptance goes one
 * family at a time: each takes the family row's lock, and a refusal (a third
 * account) stays on its own card instead of failing the batch.
 */
export function useFamilySuggestions(): FamilySuggestions {
  const [suggestions, setSuggestions] = useState<FamilySuggestion[]>([]);
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState<ReadonlySet<string>>(new Set());
  const [errors, setErrors] = useState<Record<string, string>>({});
  const inFlight = useRef(new Set<string>());
  const bulk = useRef(false);
  const [bulkRunning, setBulkRunning] = useState(false);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    let alive = true;
    listFamilySuggestions(API_URL)
      .then((rows) => { if (alive) { setSuggestions(rows); setFailed(false); } })
      .catch(() => { if (alive) setFailed(true); });
    return () => { alive = false; };
  }, [version]);
  const reload = useCallback(() => setVersion((v) => v + 1), []);

  const drop = useCallback((signature: string) => setSuggestions((prev) => prev.filter((s) => s.signature !== signature)), []);

  const run = useCallback(async (s: FamilySuggestion, write: () => Promise<unknown>, fallback: string): Promise<boolean> => {
    if (inFlight.current.has(s.signature)) return false;
    inFlight.current.add(s.signature);
    setBusy(new Set(inFlight.current));
    setErrors((prev) => ({ ...prev, [s.signature]: "" }));
    try {
      await write();
      drop(s.signature);
      return true;
    } catch (cause) {
      setErrors((prev) => ({ ...prev, [s.signature]: familyRefusal(cause, fallback) }));
      return false;
    } finally {
      inFlight.current.delete(s.signature);
      setBusy(new Set(inFlight.current));
    }
  }, [drop]);

  const build = useCallback(
    (s: FamilySuggestion) => run(s, () => createFamily(API_URL, { name: s.proposedName, studentIds: s.studentIds }), "No se pudo crear la familia."),
    [run],
  );
  const accept = useCallback(async (s: FamilySuggestion): Promise<boolean> => {
    const ok = await build(s);
    if (ok) reload();
    return ok;
  }, [build, reload]);
  const dismiss = useCallback(
    (s: FamilySuggestion) => run(s, () => dismissFamilySuggestion(API_URL, s.studentIds), "No se pudo descartar la sugerencia."),
    [run],
  );
  const acceptAllHigh = useCallback(async (): Promise<number> => {
    if (bulk.current) return 0;
    bulk.current = true;
    setBulkRunning(true);
    let built = 0;
    try {
      for (const s of suggestions.filter((x) => x.confidence === "high")) {
        if (await build(s)) built += 1;
      }
    } finally {
      bulk.current = false;
      setBulkRunning(false);
    }
    if (built > 0) reload();
    return built;
  }, [build, reload, suggestions]);

  return { suggestions, failed, busy, errors, accept, dismiss, acceptAllHigh, drop, reload, bulkRunning };
}

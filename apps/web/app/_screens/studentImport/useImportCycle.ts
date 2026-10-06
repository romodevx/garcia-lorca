"use client";

import { useCallback, useEffect, useState } from "react";

import { activateCycle, createCycle, listCycles, type CreateCycleInput, type Cycle } from "../../config/api";
import { cycleFailureMessage } from "../config/CycleBar";

export interface ImportCycle {
  state: "loading" | "failed" | "ready";
  /** The cycle the pupils will enter; undefined means step 0 is needed. */
  active: Cycle | undefined;
  /** A cycle that exists but is not active: offered for activation instead of a new one. */
  dormant: Cycle | undefined;
  pending: boolean;
  error: string | undefined;
  createAndActivate: (input: CreateCycleInput) => Promise<void>;
  activate: (id: string) => Promise<void>;
  reload: () => void;
}

/** The newest draft by start date: the year the school was setting up. */
function newestDraft(cycles: readonly Cycle[]): Cycle | undefined {
  return cycles.filter((c) => c.status === "draft").sort((a, b) => b.startsOn.localeCompare(a.startsOn))[0];
}

/** Step 0 (O9): the active cycle, or creating/activating one. Reuses the config endpoints. */
export function useImportCycle(baseURL: string): ImportCycle {
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [state, setState] = useState<ImportCycle["state"]>("loading");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [round, setRound] = useState(0);

  useEffect(() => {
    let live = true;
    listCycles(baseURL)
      .then((list) => {
        if (!live) return;
        setCycles(list);
        setState("ready");
      })
      .catch(() => live && setState("failed"));
    return () => {
      live = false;
    };
  }, [baseURL, round]);

  const settle = useCallback(async (work: () => Promise<Cycle>): Promise<void> => {
    setPending(true);
    setError(undefined);
    try {
      const activated = await work();
      setCycles((list) => [...list.filter((c) => c.id !== activated.id).map((c) => (c.status === "active" ? { ...c, status: "closed" as const } : c)), { ...activated, status: "active" }]);
    } catch (err) {
      setError(cycleFailureMessage(err));
    } finally {
      setPending(false);
    }
  }, []);

  const createAndActivate = useCallback(
    (input: CreateCycleInput) => settle(async () => {
      const created = await createCycle(baseURL, input);
      return activateCycle(baseURL, created.id);
    }),
    [baseURL, settle],
  );
  const activate = useCallback((id: string) => settle(() => activateCycle(baseURL, id)), [baseURL, settle]);
  const reload = useCallback(() => setRound((n) => n + 1), []);

  return { state, active: cycles.find((c) => c.status === "active"), dormant: newestDraft(cycles), pending, error, createAndActivate, activate, reload };
}

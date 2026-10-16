"use client";

import { useCallback, useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { listCycles, listGradeLevels, listLabelledGroups, listLevels, type GradeLevel, type LabelledGroup, type Level } from "../../config/api";

/** What the alta offers to choose from: the school's levels, and the active cycle's groups. */
export interface AltaCatalog {
  levels: readonly Level[];
  /** The active cycle's groups; the page offers the chosen grade's. */
  groups: readonly LabelledGroup[];
  /** The active cycle's first day: the earliest entry day the server takes. */
  cycleStartsOn: string;
  status: "loading" | "ready" | "failed";
  retry: () => void;
}

/**
 * Reads the catalog the alta needs once per visit. Groups that cannot be read
 * leave the picker on Automático, which the server resolves anyway; levels
 * that cannot be read are a failure, since nobody can be placed without one.
 */
export function useAltaCatalog(): AltaCatalog {
  const [levels, setLevels] = useState<Level[]>([]);
  const [groups, setGroups] = useState<LabelledGroup[]>([]);
  const [cycleStartsOn, setCycleStartsOn] = useState("");
  const [status, setStatus] = useState<AltaCatalog["status"]>("loading");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let alive = true;
    setStatus("loading");
    listLevels(API_URL)
      .then((ls) => { if (alive) { setLevels(ls); setStatus("ready"); } })
      .catch(() => { if (alive) setStatus("failed"); });
    listCycles(API_URL)
      .then(async (cs) => {
        const active = cs.find((c) => c.status === "active") ?? cs[0];
        if (active === undefined || !alive) return;
        setCycleStartsOn(active.startsOn);
        const gs = await listLabelledGroups(API_URL, active.id);
        if (alive) setGroups(gs);
      })
      .catch(() => { if (alive) setGroups([]); });
    return () => { alive = false; };
  }, [attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  return { levels, groups, cycleStartsOn, status, retry };
}

/**
 * The chosen level's grades: whatever the school configured — kínder has three,
 * primaria six — so a school that adds one sees it without anybody editing this.
 */
export function useGradesOf(levelId: string): readonly GradeLevel[] {
  const [grades, setGrades] = useState<GradeLevel[]>([]);
  useEffect(() => {
    if (levelId === "") {
      setGrades([]);
      return;
    }
    let alive = true;
    listGradeLevels(API_URL, levelId)
      .then((gs) => { if (alive) setGrades(gs); })
      .catch(() => { if (alive) setGrades([]); });
    return () => { alive = false; };
  }, [levelId]);
  return grades;
}

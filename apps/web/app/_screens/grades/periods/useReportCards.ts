import { useEffect, useState } from "react";

import { API_URL } from "../../../api-url";
import { listGradeLevels, listLabelledGroups, type LabelledGroup } from "../../../config/api";
import type { PeriodsClient, ReportCard } from "../../../grades/periods";

export type Loadable<T> = { kind: "idle" } | { kind: "loading" } | { kind: "ready"; value: T } | { kind: "failed" };

/** The level's groups in the cycle — a boleta list is per group. */
export function useLevelGroups(cycleId: string, levelId: string): Loadable<LabelledGroup[]> {
  const [state, setState] = useState<Loadable<LabelledGroup[]>>({ kind: "loading" });
  useEffect(() => {
    let active = true;
    Promise.all([listLabelledGroups(API_URL, cycleId), listGradeLevels(API_URL, levelId)])
      .then(([groups, grades]) => {
        const ofLevel = new Set(grades.map((g) => g.id));
        if (active) setState({ kind: "ready", value: groups.filter((g) => ofLevel.has(g.gradeLevelId)) });
      })
      .catch(() => active && setState({ kind: "failed" }));
    return () => {
      active = false;
    };
  }, [cycleId, levelId]);
  return state;
}

/** One group's boletas for the term; idle until a group is chosen. */
export function useReportCards(client: PeriodsClient, termId: string, groupId: string): Loadable<ReportCard[]> {
  const [state, setState] = useState<Loadable<ReportCard[]>>({ kind: "idle" });
  useEffect(() => {
    if (groupId === "") {
      setState({ kind: "idle" });
      return;
    }
    let active = true;
    setState({ kind: "loading" });
    client
      .reportCards(termId, groupId)
      .then((value) => active && setState({ kind: "ready", value }))
      .catch(() => active && setState({ kind: "failed" }));
    return () => {
      active = false;
    };
  }, [client, termId, groupId]);
  return state;
}

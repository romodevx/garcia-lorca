import { useCallback, useEffect, useState } from "react";

import { API_URL } from "../../../api-url";
import { listCycles, type Cycle } from "../../../config/api";
import type { GradingPeriod, PeriodsClient } from "../../../grades/periods";

export type PeriodsLoad =
  | { kind: "loading" }
  | { kind: "ready"; cycle: Cycle; periods: GradingPeriod[] }
  | { kind: "noCycle" }
  | { kind: "failed" };

/** The cycle whose periods matter: the active one, else the latest that exists. */
function currentCycle(cycles: readonly Cycle[]): Cycle | undefined {
  return cycles.find((c) => c.status === "active") ?? [...cycles].sort((a, b) => b.startsOn.localeCompare(a.startsOn))[0];
}

/** The current cycle's grading periods; `reload` after a close or a reopen. */
export function usePeriods(client: PeriodsClient): { load: PeriodsLoad; reload: () => void } {
  const [load, setLoad] = useState<PeriodsLoad>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    (async (): Promise<PeriodsLoad> => {
      const cycle = currentCycle(await listCycles(API_URL));
      if (cycle === undefined) return { kind: "noCycle" };
      return { kind: "ready", cycle, periods: await client.list(cycle.id) };
    })()
      .then((next) => active && setLoad(next))
      .catch(() => active && setLoad({ kind: "failed" }));
    return () => {
      active = false;
    };
  }, [client, attempt]);

  const reload = useCallback(() => setAttempt((n) => n + 1), []);
  return { load, reload };
}

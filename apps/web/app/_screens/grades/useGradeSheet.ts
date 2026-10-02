import { useCallback, useEffect, useRef, useState } from "react";

import { isRejectedValue, type GradeSheetClient, type PeriodSheet } from "../../grades/periodSheet";
import type { GradeSaveStore } from "./gradeSaveStore";

export type SheetLoad =
  | { kind: "loading" }
  | { kind: "ready"; sheet: PeriodSheet }
  /** The server explained why there is no sheet (e.g. preescolar has no numeric grade). */
  | { kind: "unavailable"; reason: string }
  | { kind: "failed" };

export interface GradeSheetState {
  load: SheetLoad;
  /** The last sheet that arrived — keeps the period chips on screen while the next loads. */
  lastSheet: PeriodSheet | undefined;
  /** The period asked for; undefined until the teacher picks one (the server chooses). */
  periodId: string | undefined;
  choosePeriod: (periodId: string) => void;
  reload: () => void;
}

/**
 * One assignment's sheet for the chosen period. Every request is numbered and
 * only the newest may paint, so a slow answer for a period the teacher already
 * left can never overwrite the one they are looking at. The GET waits for that
 * period's saves still in flight, so it never brings back a value just replaced;
 * the loaded rows then become the truth for every clean cell (`adopt`).
 */
export function useGradeSheet(client: GradeSheetClient, assignmentId: string, saves: GradeSaveStore): GradeSheetState {
  const [periodId, setPeriodId] = useState<string | undefined>(undefined);
  const [load, setLoad] = useState<SheetLoad>({ kind: "loading" });
  const [lastSheet, setLastSheet] = useState<PeriodSheet | undefined>(undefined);
  const [attempt, setAttempt] = useState(0);
  const latest = useRef(0);

  useEffect(() => {
    const request = ++latest.current;
    setLoad({ kind: "loading" });
    const scope = periodId === undefined ? { assignmentId } : { assignmentId, periodId };
    saves
      .settled(scope)
      .then(() => (request === latest.current ? client.load(scope) : undefined))
      .then((sheet) => {
        if (sheet === undefined || request !== latest.current) return;
        saves.adopt(sheet);
        setLastSheet(sheet);
        setLoad({ kind: "ready", sheet });
      })
      .catch((err: unknown) => {
        if (request !== latest.current) return;
        setLoad(isRejectedValue(err) ? { kind: "unavailable", reason: err.detail } : { kind: "failed" });
      });
  }, [client, assignmentId, periodId, attempt, saves]);

  const reload = useCallback(() => setAttempt((n) => n + 1), []);

  return { load, lastSheet, periodId, choosePeriod: setPeriodId, reload };
}

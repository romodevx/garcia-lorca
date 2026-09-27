import { useCallback, useEffect, useRef, useState } from "react";

import { isRejectedValue, type GradeSheet, type GradeSheetClient } from "../../grades/monthly";
import type { GradeSaveStore } from "./gradeSaveStore";

export type SheetLoad =
  | { kind: "loading" }
  | { kind: "ready"; sheet: GradeSheet }
  /** The server explained why there is no sheet (e.g. preescolar has no numeric grade). */
  | { kind: "unavailable"; reason: string }
  | { kind: "failed" };

export interface GradeSheetState {
  load: SheetLoad;
  /** The last sheet that arrived — keeps the month chips on screen while the next loads. */
  lastSheet: GradeSheet | undefined;
  /** The month asked for; undefined until the teacher picks one (the server chooses). */
  month: string | undefined;
  chooseMonth: (month: string) => void;
  reload: () => void;
}

/**
 * One assignment's sheet for the chosen month. Every request is numbered and
 * only the newest may paint, so a slow answer for a month the teacher already
 * left can never overwrite the one they are looking at. The GET waits for that
 * month's saves still in flight, so it never brings back a value just replaced;
 * the loaded rows then become the truth for every clean cell (`adopt`).
 */
export function useGradeSheet(client: GradeSheetClient, assignmentId: string, saves: GradeSaveStore): GradeSheetState {
  const [month, setMonth] = useState<string | undefined>(undefined);
  const [load, setLoad] = useState<SheetLoad>({ kind: "loading" });
  const [lastSheet, setLastSheet] = useState<GradeSheet | undefined>(undefined);
  const [attempt, setAttempt] = useState(0);
  const latest = useRef(0);

  useEffect(() => {
    const request = ++latest.current;
    setLoad({ kind: "loading" });
    saves
      .settled(month === undefined ? { assignmentId } : { assignmentId, month })
      .then(() => (request === latest.current ? client.load(month === undefined ? { assignmentId } : { assignmentId, month }) : undefined))
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
  }, [client, assignmentId, month, attempt, saves]);

  const reload = useCallback(() => setAttempt((n) => n + 1), []);

  return { load, lastSheet, month, chooseMonth: setMonth, reload };
}

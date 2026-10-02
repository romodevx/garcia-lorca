/**
 * The period grade sheet over HTTP. The teacher (`/teaching/grades/*`) and
 * the director (`/grades/*`) speak the same contract under different roots,
 * so one factory serves both. Rejections carry the server's text/plain reason
 * (`GradeRequestError`) so the sheet can show it on the row it belongs to.
 */

import { authFetch } from "../auth/authFetch";
import {
  gradeKeySearch,
  sheetSearch,
  type GradeSheetClient,
  type PeriodGradeKey,
  type PeriodSheet,
  type SavedPeriodGrade,
  type SavePeriodGradeInput,
} from "./periodSheet";
import { gradeFailure } from "./requestError";

/** `root` is the absolute prefix, e.g. `http://gw/api/v1/teaching/grades`. */
export function createGradeSheetClient(root: string): GradeSheetClient {
  const send = async (path: string, init: RequestInit = {}): Promise<Response> => {
    const res = await authFetch(`${root}${path}`, { credentials: "include", ...init });
    if (!res.ok) throw await gradeFailure(res);
    return res;
  };

  return {
    load: async (q) => (await (await send(`/period-sheet${sheetSearch(q)}`)).json()) as PeriodSheet,
    save: async (input: SavePeriodGradeInput) => {
      const body = { assignmentId: input.assignmentId, periodId: input.periodId, enrollmentId: input.enrollmentId, value: input.value };
      const res = await send("/period-grades", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      return (await res.json()) as SavedPeriodGrade;
    },
    remove: async (key: PeriodGradeKey) => {
      await send(`/period-grades${gradeKeySearch(key)}`, { method: "DELETE" });
    },
  };
}

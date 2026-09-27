/**
 * The monthly grade sheet over HTTP. The teacher (`/teaching/grades/*`) and
 * the director (`/grades/*`) speak the same contract under different roots,
 * so one factory serves both. Rejections carry the server's text/plain reason
 * (`GradeRequestError`) so the sheet can show it on the row it belongs to.
 */

import { authFetch } from "../auth/authFetch";
import {
  gradeFailure,
  gradeKeySearch,
  sheetSearch,
  type GradeSheet,
  type GradeSheetClient,
  type MonthlyGradeKey,
  type SaveMonthlyGradeInput,
  type SavedMonthlyGrade,
} from "./monthly";

/** `root` is the absolute prefix, e.g. `http://gw/api/v1/teaching/grades`. */
export function createGradeSheetClient(root: string): GradeSheetClient {
  const send = async (path: string, init: RequestInit = {}): Promise<Response> => {
    const res = await authFetch(`${root}${path}`, { credentials: "include", ...init });
    if (!res.ok) throw await gradeFailure(res);
    return res;
  };

  return {
    load: async (q) => (await (await send(`/sheet${sheetSearch(q)}`)).json()) as GradeSheet,
    save: async (input: SaveMonthlyGradeInput) => {
      const body = { assignmentId: input.assignmentId, month: input.month, enrollmentId: input.enrollmentId, value: input.value };
      const res = await send("/monthly", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      return (await res.json()) as SavedMonthlyGrade;
    },
    remove: async (key: MonthlyGradeKey) => {
      await send(`/monthly${gradeKeySearch(key)}`, { method: "DELETE" });
    },
  };
}

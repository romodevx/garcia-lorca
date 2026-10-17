import type { Cycle } from "../../config/api";

/** The month a Mexican school year opens: it runs August to July. */
const FIRST_MONTH = 8;
const FIRST_DAY = "08-01";
const LAST_DAY = "07-31";

export interface CycleProposal {
  name: string;
  startsOn: string;
  endsOn: string;
}

/** The school year that opens in August of `year`. */
function schoolYear(year: number): CycleProposal {
  return { name: `${year}-${year + 1}`, startsOn: `${year}-${FIRST_DAY}`, endsOn: `${year + 1}-${LAST_DAY}` };
}

/**
 * What the "Nuevo ciclo" drawer opens with (ISSUE-T14-023), so the happy path
 * is one click: the year after the latest cycle, or — for a school's first
 * cycle — the school year `today` (aaaa-mm-dd, school time) falls in. The
 * dates are text, so the year and month are read from it: no Date, no zone.
 */
export function proposeCycle(cycles: readonly Cycle[], today: string): CycleProposal {
  const latestEnd = cycles.reduce((max, c) => (c.endsOn > max ? c.endsOn : max), "");
  if (latestEnd !== "") return schoolYear(Number(latestEnd.slice(0, 4)));
  const year = Number(today.slice(0, 4));
  const month = Number(today.slice(5, 7));
  return schoolYear(month >= FIRST_MONTH ? year : year - 1);
}

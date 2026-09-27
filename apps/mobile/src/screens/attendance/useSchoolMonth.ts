import { useEffect, useState } from "react";
import type { AttendanceDay, AttendanceMonth } from "../../api/attendance";
import { useChildAttendanceMonth } from "../../state/familyData";
import type { AsyncResource } from "../../state/useAsyncResource";
import { deviceMonth, findToday, monthOf } from "../attendanceView";

export interface SchoolMonthAttendance {
  /** The school's current month: the phone's guess until the server answers, then monthOf(today). */
  month: string;
  /** That month of the child's attendance. */
  resource: AsyncResource<AttendanceMonth>;
  /** Today's entry, or null when it is unknown (see {@link findToday}). */
  today: AttendanceDay | null;
}

/**
 * The child's attendance for the SCHOOL's current month. The phone's clock only
 * picks the first request; every answer carries the school's today, and when its
 * month differs (midnight rollover, the UTC school day, a phone clock that is
 * off) the read re-keys to it. Coming back to the foreground reloads the month
 * (freshness signal), so a new day re-evaluates it too.
 */
export function useSchoolMonth(childId: string | null, initialMonth?: string): SchoolMonthAttendance {
  const [month, setMonth] = useState(() => initialMonth ?? deviceMonth());
  const resource = useChildAttendanceMonth(childId, month);
  const serverToday = resource.data?.today ?? "";
  const schoolMonth = serverToday === "" ? null : monthOf(serverToday);

  useEffect(() => {
    if (schoolMonth !== null && schoolMonth !== month) {
      setMonth(schoolMonth);
    }
  }, [schoolMonth, month]);

  return { month, resource, today: findToday(resource.data) };
}

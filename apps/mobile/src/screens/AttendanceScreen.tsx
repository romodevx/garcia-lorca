import { useState, type ReactElement } from "react";
import { useRefreshHandler } from "../components/refresh";
import { useAppState } from "../state/AppState";
import { useChildAttendanceMonth } from "../state/familyData";
import type { AsyncResource } from "../state/useAsyncResource";
import type { AttendanceMonth } from "../api/attendance";
import { ScreenTitle, SectionLabel } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";
import { DayList } from "./attendance/DayList";
import { MonthSelector, MonthSummary } from "./attendance/MonthBar";
import { TodayCard } from "./attendance/TodayCard";
import { deviceMonth, monthOf, shiftMonth } from "./attendanceView";

const HTTP_NOT_FOUND = 404;

export interface AttendanceScreenProps {
  /** The month to open on; the phone's month by default (tests pin it). */
  initialMonth?: string;
}

/**
 * Asistencia: today first (did my child attend?), then the month — summary,
 * selector and every school day. Reads
 * `GET /family/students/{id}/attendance?month=`; reloads on pull, on returning
 * to the foreground and on an attendance push. Never fabricates: a failed read
 * is an error with retry, a 404 says the child was not found.
 */
export function AttendanceScreen({ initialMonth }: AttendanceScreenProps): ReactElement {
  const { selectedChildId, childLoading, childError, refreshChildren } = useAppState();
  const [openingMonth] = useState(() => initialMonth ?? deviceMonth());
  const [selected, setSelected] = useState(openingMonth);

  const current = useChildAttendanceMonth(selectedChildId, openingMonth);
  const isCurrentSelected = selected === openingMonth;
  const other = useChildAttendanceMonth(selectedChildId, isCurrentSelected ? null : selected);
  const shown = isCurrentSelected ? current : other;

  useRefreshHandler(async () => {
    await Promise.all([current.reload(), isCurrentSelected ? Promise.resolve() : other.reload()]);
  });

  if (childError) {
    return <Failed onRetry={refreshChildren} />;
  }
  if (current.error) {
    return <Failed onRetry={current.reload} notFound={current.errorStatus === HTTP_NOT_FOUND} />;
  }
  if (current.data === null) {
    return (
      <>
        <ScreenTitle>Asistencia</ScreenTitle>
        {childLoading || current.loading ? <AsyncLoading /> : null}
      </>
    );
  }

  const today = current.data.today;
  const todayRecord = current.data.days.find((d) => d.date === today);
  const lastMonth = today ? monthOf(today) : openingMonth;

  return (
    <>
      <ScreenTitle>Asistencia</ScreenTitle>
      <TodayCard day={todayRecord} />
      <MonthSelector
        month={selected}
        canGoNext={selected < lastMonth}
        onPrevious={() => setSelected((m) => shiftMonth(m, -1))}
        onNext={() => setSelected((m) => (m < lastMonth ? shiftMonth(m, 1) : m))}
      />
      <MonthSection resource={shown} />
    </>
  );
}

function MonthSection({ resource }: { resource: AsyncResource<AttendanceMonth> }): ReactElement {
  if (resource.error) {
    return (
      <AsyncErrorState
        title="No pudimos cargar este mes"
        subtitle="Revisa tu conexión e inténtalo de nuevo."
        onRetry={resource.reload}
      />
    );
  }
  if (resource.data === null) {
    return <AsyncLoading />;
  }
  return (
    <>
      <MonthSummary summary={resource.data.summary} />
      <SectionLabel>Días</SectionLabel>
      <DayList days={resource.data.days} />
    </>
  );
}

function Failed({ onRetry, notFound = false }: { onRetry: () => void; notFound?: boolean }): ReactElement {
  return (
    <>
      <ScreenTitle>Asistencia</ScreenTitle>
      <AsyncErrorState
        title={notFound ? "No encontramos a este alumno" : "No pudimos cargar la asistencia"}
        subtitle={notFound ? "Si crees que es un error, avisa a la escuela." : "Revisa tu conexión e inténtalo de nuevo."}
        onRetry={onRetry}
      />
    </>
  );
}

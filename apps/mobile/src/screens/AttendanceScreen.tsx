import { useState, type ReactElement } from "react";
import { useRefreshHandler } from "../components/refresh";
import { useAppState } from "../state/AppState";
import { useChildAttendanceMonth } from "../state/familyData";
import type { AsyncResource } from "../state/useAsyncResource";
import type { AttendanceMonth } from "../api/attendance";
import { ScreenTitle, SectionLabel } from "../ui";
import { AsyncErrorState, AsyncLoading, StaleNotice } from "./AsyncStates";
import { DayList } from "./attendance/DayList";
import { MonthSelector, MonthSummary } from "./attendance/MonthBar";
import { TodayCard } from "./attendance/TodayCard";
import { shiftMonth } from "./attendanceView";
import { useSchoolMonth } from "./attendance/useSchoolMonth";

const HTTP_NOT_FOUND = 404;

export interface AttendanceScreenProps {
  /** The month to ask for first; the phone's by default (tests pin it). The school's today then decides. */
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
  const { month: currentMonth, resource: current, today } = useSchoolMonth(selectedChildId, initialMonth);
  // null = "the current month"; a picked month is only honoured while it is in the past.
  const [picked, setPicked] = useState<string | null>(null);
  const selected = picked !== null && picked < currentMonth ? picked : currentMonth;

  const isCurrentSelected = selected === currentMonth;
  const other = useChildAttendanceMonth(selectedChildId, isCurrentSelected ? null : selected);
  const shown = isCurrentSelected ? current : other;

  useRefreshHandler(async () => {
    await Promise.all([current.reload(), isCurrentSelected ? Promise.resolve() : other.reload()]);
  });

  if (childError) {
    return <Failed onRetry={refreshChildren} />;
  }
  if (current.error && current.data === null) {
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

  const stale = current.error || (shown.error && shown.data !== null);
  const goNext = (): void => {
    const next = shiftMonth(selected, 1);
    setPicked(next < currentMonth ? next : null);
  };

  return (
    <>
      <ScreenTitle>Asistencia</ScreenTitle>
      {stale ? <StaleNotice /> : null}
      <TodayCard day={today} />
      <MonthSelector
        month={selected}
        canGoNext={selected < currentMonth}
        onPrevious={() => setPicked(shiftMonth(selected, -1))}
        onNext={goNext}
      />
      <MonthSection resource={shown} />
    </>
  );
}

function MonthSection({ resource }: { resource: AsyncResource<AttendanceMonth> }): ReactElement {
  if (resource.error && resource.data === null) {
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

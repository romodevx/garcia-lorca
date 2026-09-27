import { useState, type ReactElement } from "react";
import { useRefreshHandler } from "../components/refresh";
import { useAppState } from "../state/AppState";
import { toChildView } from "../state/childView";
import { useChildMonthlyGrades } from "../state/familyData";
import { Card, EmptyState, ScreenTitle } from "../ui";
import { deviceMonth } from "./attendanceView";
import { AsyncErrorState, AsyncLoading, StaleNotice } from "./AsyncStates";
import { MonthChips } from "./grades/MonthChips";
import { MonthGrades } from "./grades/MonthGrades";
import { defaultMonth, hasCapturedGrades } from "./gradesView";

const HTTP_NOT_FOUND = 404;
const TITLE = "Calificaciones";

export interface GradesScreenProps {
  /** The phone's month by default (tests pin it); only a fallback for the first month shown. */
  currentMonth?: string;
}

/** A month the parent picked, remembered only for the child it was picked on. */
interface PickedMonth {
  childId: string | null;
  month: string;
}

/**
 * Calificaciones: the child's monthly grade per subject, visible as soon as the
 * teacher captures it (`GET /family/students/{id}/grades/monthly`). Month chips,
 * the month's average, and each subject's grade — a failing one says
 * "Reprobada" in words, never by colour alone. A level without numeric grades
 * shows what was captured before it stopped (as stored, judged by no scale);
 * only with nothing captured does it keep the qualitative "Próximamente"
 * placeholder. Reloads on pull and on foreground.
 */
export function GradesScreen({ currentMonth }: GradesScreenProps): ReactElement {
  const { child, selectedChildId, childLoading, childError, refreshChildren } = useAppState();
  const { data, loading, error, errorStatus, reload } = useChildMonthlyGrades(selectedChildId);
  const [picked, setPicked] = useState<PickedMonth | null>(null);
  useRefreshHandler(reload);

  if (childError) {
    return <Failed onRetry={refreshChildren} />;
  }
  const withoutNumericGrades = toChildView(child).qualitative || data?.gradingEnabled === false;
  if (withoutNumericGrades && (data === null || !hasCapturedGrades(data))) {
    return <QualitativePending />;
  }
  if (error && data === null) {
    return <Failed onRetry={reload} notFound={errorStatus === HTTP_NOT_FOUND} />;
  }
  if (data === null) {
    return (
      <>
        <ScreenTitle>{TITLE}</ScreenTitle>
        {childLoading || loading ? <AsyncLoading /> : null}
      </>
    );
  }

  const pickedMonth = picked?.childId === selectedChildId ? picked.month : null;
  const month =
    pickedMonth !== null && data.months.includes(pickedMonth) ? pickedMonth : defaultMonth(data, currentMonth ?? deviceMonth());

  return (
    <>
      <ScreenTitle>{TITLE}</ScreenTitle>
      {error ? <StaleNotice /> : null}
      {data.subjects.length === 0 ? (
        <Card padded style={{ marginTop: 12 }}>
          <EmptyState
            icon="book"
            title="Aún no hay calificaciones"
            subtitle="Aquí verás las materias y su calificación de cada mes cuando la escuela las capture."
          />
        </Card>
      ) : (
        <>
          <MonthChips
            months={data.months}
            selected={month}
            onSelect={(m) => setPicked({ childId: selectedChildId, month: m })}
          />
          <MonthGrades subjects={data.subjects} month={month} scale={data.scale} />
        </>
      )}
    </>
  );
}

function QualitativePending(): ReactElement {
  return (
    <>
      <ScreenTitle>{TITLE}</ScreenTitle>
      <Card padded style={{ marginTop: 12 }}>
        <EmptyState
          icon="sparkles"
          title="Próximamente"
          subtitle="La boleta cualitativa de este nivel estará disponible pronto."
        />
      </Card>
    </>
  );
}

function Failed({ onRetry, notFound = false }: { onRetry: () => void; notFound?: boolean }): ReactElement {
  return (
    <>
      <ScreenTitle>{TITLE}</ScreenTitle>
      <AsyncErrorState
        title={notFound ? "No encontramos a este alumno" : "No pudimos cargar las calificaciones"}
        subtitle={notFound ? "Si crees que es un error, avisa a la escuela." : "Revisa tu conexión e inténtalo de nuevo."}
        onRetry={onRetry}
      />
    </>
  );
}

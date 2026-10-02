import { useState, type ReactElement } from "react";
import { useRefreshHandler } from "../../components/refresh";
import { useAppState } from "../../state/AppState";
import { toChildView } from "../../state/childView";
import { useChildPeriodGrades } from "../../state/familyData";
import { Card, EmptyState } from "../../ui";
import { AsyncErrorState, AsyncLoading, StaleNotice } from "../AsyncStates";
import { defaultPeriod, deviceDay, hasCapturedGrades } from "../gradesView";
import { PeriodChips } from "./PeriodChips";
import { PeriodGrades } from "./PeriodGrades";

const HTTP_NOT_FOUND = 404;

export interface PeriodGradesViewProps {
  /** The phone's LOCAL date (YYYY-MM-DD) by default (tests pin it); picks the first period shown. */
  today?: string;
}

/** A period the parent picked, remembered only for the child it was picked on. */
interface PickedPeriod {
  childId: string | null;
  periodId: string;
}

/**
 * "Por periodo": the child's grade per subject in each evaluation period of
 * the child's grade, visible as soon as the teacher captures it
 * (`GET /family/students/{id}/grades/periods`). Period chips (today's period
 * first), the general average, and each subject's grade and average — a
 * failing grade says "Reprobada" in words, never by colour alone; tapping a
 * subject opens its grade in every period. A level without numeric grades shows what was captured before it
 * stopped (as stored, judged by no scale); only with nothing captured does it
 * keep the qualitative "Próximamente" placeholder. Reloads on pull and on
 * foreground.
 */
export function PeriodGradesView({ today }: PeriodGradesViewProps): ReactElement | null {
  const { child, selectedChildId, childLoading } = useAppState();
  const { data, loading, error, errorStatus, reload } = useChildPeriodGrades(selectedChildId);
  const [picked, setPicked] = useState<PickedPeriod | null>(null);
  useRefreshHandler(reload);

  const withoutNumericGrades = toChildView(child).qualitative || data?.gradingEnabled === false;
  if (withoutNumericGrades && (data === null || !hasCapturedGrades(data.subjects))) {
    return <QualitativePending />;
  }
  if (error && data === null) {
    return <Failed onRetry={reload} notFound={errorStatus === HTTP_NOT_FOUND} />;
  }
  if (data === null) {
    return childLoading || loading ? <AsyncLoading /> : null;
  }

  const pickedId = picked?.childId === selectedChildId ? picked.periodId : null;
  const period =
    data.periods.find((p) => p.id === pickedId) ??
    data.periods.find((p) => p.id === defaultPeriod(data.periods, today ?? deviceDay()));

  return (
    <>
      {error ? <StaleNotice /> : null}
      {data.subjects.length === 0 || period === undefined ? (
        <Card padded style={{ marginTop: 12 }}>
          <EmptyState
            icon="book"
            title="Aún no hay calificaciones"
            subtitle="Aquí verás las materias y su calificación de cada periodo cuando la escuela las capture."
          />
        </Card>
      ) : (
        <>
          <PeriodChips
            periods={data.periods}
            selected={period.id}
            onSelect={(id) => setPicked({ childId: selectedChildId, periodId: id })}
          />
          <PeriodGrades subjects={data.subjects} period={period} periods={data.periods} scale={data.scale} />
        </>
      )}
    </>
  );
}

function QualitativePending(): ReactElement {
  return (
    <Card padded style={{ marginTop: 12 }}>
      <EmptyState
        icon="sparkles"
        title="Próximamente"
        subtitle="La boleta cualitativa de este nivel estará disponible pronto."
      />
    </Card>
  );
}

function Failed({ onRetry, notFound }: { onRetry: () => void; notFound: boolean }): ReactElement {
  return (
    <AsyncErrorState
      title={notFound ? "No encontramos a este alumno" : "No pudimos cargar las calificaciones"}
      subtitle={notFound ? "Si crees que es un error, avisa a la escuela." : "Revisa tu conexión e inténtalo de nuevo."}
      onRetry={onRetry}
    />
  );
}

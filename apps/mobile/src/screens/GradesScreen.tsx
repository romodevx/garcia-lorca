import { useEffect, useState, type ReactElement } from "react";
import { StyleSheet, View } from "react-native";
import { useAppState } from "../state/AppState";
import { ScreenTitle, Segment, type SegmentOption } from "../ui";
import { AsyncErrorState } from "./AsyncStates";
import { MonthlyGradesView } from "./grades/MonthlyGradesView";
import { ReportCardsView } from "./grades/ReportCardsView";

export type GradesView = "mes" | "boletas";

const VIEWS: ReadonlyArray<SegmentOption<GradesView>> = [
  { value: "mes", label: "Por mes" },
  { value: "boletas", label: "Boletas" },
];

export interface GradesScreenProps {
  /** The phone's month by default (tests pin it); only a fallback for the first month shown. */
  currentMonth?: string;
  /** A segment the route asks for (a boleta push opens "boletas"). */
  requestedView?: GradesView;
  /** Changes on every new request, so asking for the same segment again re-applies it. */
  requestKey?: string;
}

/**
 * Calificaciones: two segments — "Por mes" (each month's grade per subject, as
 * captured) and "Boletas" (the issued trimester report cards).
 */
export function GradesScreen({ currentMonth, requestedView, requestKey }: GradesScreenProps): ReactElement {
  const { childError, refreshChildren } = useAppState();
  const [view, setView] = useState<GradesView>(requestedView ?? "mes");

  useEffect(() => {
    if (requestedView !== undefined) {
      setView(requestedView);
    }
  }, [requestedView, requestKey]);

  return (
    <>
      <ScreenTitle>Calificaciones</ScreenTitle>
      <View style={styles.segment}>
        <Segment options={VIEWS} value={view} onChange={setView} />
      </View>
      {childError ? (
        <AsyncErrorState
          title="No pudimos cargar las calificaciones"
          subtitle="Revisa tu conexión e inténtalo de nuevo."
          onRetry={refreshChildren}
        />
      ) : view === "mes" ? (
        <MonthlyGradesView currentMonth={currentMonth} />
      ) : (
        <ReportCardsView />
      )}
    </>
  );
}

const styles = StyleSheet.create({
  segment: { marginTop: 12 },
});

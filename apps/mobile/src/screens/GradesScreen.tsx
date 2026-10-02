import { useEffect, useState, type ReactElement } from "react";
import { StyleSheet, View } from "react-native";
import { useAppState } from "../state/AppState";
import { ScreenTitle, Segment, type SegmentOption } from "../ui";
import { AsyncErrorState } from "./AsyncStates";
import { PeriodGradesView } from "./grades/PeriodGradesView";
import { ReportCardsView } from "./grades/ReportCardsView";

export type GradesView = "periodo" | "boletas";

const VIEWS: ReadonlyArray<SegmentOption<GradesView>> = [
  { value: "periodo", label: "Por periodo" },
  { value: "boletas", label: "Boletas" },
];

export interface GradesScreenProps {
  /** The phone's LOCAL date (YYYY-MM-DD) by default (tests pin it); picks the first period shown. */
  today?: string;
  /** A segment the route asks for (a boleta push opens "boletas"). */
  requestedView?: GradesView;
  /** Changes on every new request, so asking for the same segment again re-applies it. */
  requestKey?: string;
}

/**
 * Calificaciones: two segments — "Por periodo" (each evaluation period's grade
 * per subject, as captured, with the averages) and "Boletas" (the issued
 * report cards).
 */
export function GradesScreen({ today, requestedView, requestKey }: GradesScreenProps): ReactElement {
  const { childError, refreshChildren } = useAppState();
  const [view, setView] = useState<GradesView>(requestedView ?? "periodo");

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
      ) : view === "periodo" ? (
        <PeriodGradesView today={today} />
      ) : (
        <ReportCardsView />
      )}
    </>
  );
}

const styles = StyleSheet.create({
  segment: { marginTop: 12 },
});

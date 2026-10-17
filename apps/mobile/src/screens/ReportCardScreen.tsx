import { useMemo, type ReactElement } from "react";
import { StyleSheet, Text, View } from "react-native";
import { reportCardPdfPath } from "../api/reportCards";
import { fallbackPdfName } from "../files/reportCardPdf";
import { useAppState } from "../state/AppState";
import { useChildReportCard } from "../state/familyData";
import { useTheme } from "../theme";
import { Button, Card, EmptyState, ScreenTitle } from "../ui";
import { AsyncErrorState, AsyncLoading } from "./AsyncStates";
import { formatAverage } from "./gradesView";
import { ReportCardTable } from "./grades/ReportCardTable";
import { reportCardTitle, versionBadge } from "./grades/reportCardView";
import { usePdfDownload, type PdfDownloadState } from "./grades/usePdfDownload";

const HTTP_NOT_FOUND = 404;
const PERCENT = 100;

export interface ReportCardScreenProps {
  id: string;
  /** The child the boleta belongs to (route param). Absent → the selected child. */
  studentId?: string;
}

/**
 * One issued boleta of the child named in the route (a push can open a
 * sibling's boleta, and the selection may change behind this screen): the subject × month table with each
 * average and the general average, and a primary "Descargar PDF" that saves the
 * official PDF and opens the phone's share / preview sheet.
 */
export function ReportCardScreen({ id, studentId }: ReportCardScreenProps): ReactElement | null {
  const theme = useTheme();
  const { selectedChildId, childLoading } = useAppState();
  const childId = studentId || selectedChildId;
  const { data, loading, error, errorStatus, reload } = useChildReportCard(childId, id);
  const target = useMemo(
    () =>
      data === null || childId === null
        ? null
        : {
            path: reportCardPdfPath(childId, data.id),
            kind: "boleta",
            studentId: childId,
            fallbackName: fallbackPdfName(data),
            title: `Boleta ${data.termName}`,
          },
    [data, childId],
  );
  const { state, start } = usePdfDownload(target);

  if (error && data === null) {
    const notFound = errorStatus === HTTP_NOT_FOUND;
    return (
      <AsyncErrorState
        title={notFound ? "Esta boleta ya no está disponible" : "No pudimos cargar la boleta"}
        subtitle={notFound ? "La escuela la retiró o publicó una versión nueva." : "Revisa tu conexión e inténtalo de nuevo."}
        onRetry={reload}
      />
    );
  }
  if (data === null) {
    return childLoading || loading ? <AsyncLoading /> : null;
  }

  const badge = versionBadge(data.version);
  const subtitle = [data.cycleName, badge].filter(Boolean).join(" · ");
  return (
    <>
      <ScreenTitle>{data.termName}</ScreenTitle>
      {subtitle ? <Text style={[styles.sub, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>{subtitle}</Text> : null}
      {data.hasGrades ? (
        <>
          <ReportCardTable card={data} />
          <Card style={styles.average}>
            <View accessible accessibilityLabel={`Promedio general: ${data.generalAverage === null ? "sin calificación" : formatAverage(data.generalAverage)}`} style={styles.averageRow}>
              <Text style={[styles.averageLabel, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>Promedio general</Text>
              <Text style={[styles.averageValue, { color: theme.colors.text, fontFamily: theme.fonts.uiExtra }]}>
                {data.generalAverage === null ? "—" : formatAverage(data.generalAverage)}
              </Text>
            </View>
          </Card>
        </>
      ) : (
        <Card padded style={styles.average}>
          <EmptyState icon="book" title="Sin calificaciones capturadas en este periodo" subtitle={reportCardTitle(data)} />
        </Card>
      )}
      <DownloadAction state={state} onPress={start} />
    </>
  );
}

function DownloadAction({ state, onPress }: { state: PdfDownloadState; onPress: () => void }): ReactElement {
  const theme = useTheme();
  const downloading = state.status === "downloading";
  return (
    <View style={styles.action}>
      <Button label="Descargar PDF" icon="download" onPress={onPress} />
      {downloading ? (
        <Text accessibilityLiveRegion="polite" style={[styles.status, { color: theme.colors.text2, fontFamily: theme.fonts.uiSemi }]}>
          {state.progress === null ? "Descargando…" : `Descargando… ${Math.round(state.progress * PERCENT)} %`}
        </Text>
      ) : null}
      {state.status === "failed" ? (
        <Text accessibilityRole="alert" style={[styles.status, { color: theme.colors.bad, fontFamily: theme.fonts.uiSemi }]}>
          {state.message}
        </Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  sub: { fontSize: 16, fontWeight: "600", marginTop: 4 },
  average: { marginTop: 14, padding: 18 },
  averageRow: { flexDirection: "row", alignItems: "baseline", justifyContent: "space-between" },
  averageLabel: { fontSize: 17, fontWeight: "600" },
  averageValue: { fontSize: 30, fontWeight: "800" },
  action: { marginTop: 18, gap: 10 },
  status: { fontSize: 16, fontWeight: "600", textAlign: "center" },
});

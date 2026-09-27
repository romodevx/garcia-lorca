"use client";

import { useEffect, useState, type CSSProperties, type ReactElement } from "react";

import { DrawerShell } from "../../../_design/DrawerShell";
import { Icon } from "../../../_design/Icon";
import { SelectField } from "../../../_design/form";
import { boletaFileName, type GradingPeriod, type PeriodsClient, type ReportCard } from "../../../grades/periods";
import { EmptyHint, ErrorAlert } from "../../config/parts";
import { downloadBlob } from "./downloadBlob";
import { periodTitle } from "./format";
import { useLevelGroups, useReportCards } from "./useReportCards";

export interface ReportCardsDialogProps {
  period: GradingPeriod;
  cycleId: string;
  client: PeriodsClient;
  onClose: () => void;
  /** Hands the PDF to the browser; injectable so tests never touch object URLs or timers. */
  download?: (blob: Blob, fileName: string) => void;
}

const TH: CSSProperties = { padding: "8px 6px", fontSize: 12.5, fontWeight: 700, letterSpacing: ".04em", textTransform: "uppercase", color: "var(--text-faint)", textAlign: "left" };
const TD: CSSProperties = { padding: "10px 6px", fontSize: 15, borderTop: "1px solid var(--border)" };

const average = (value: number | null): string => (value === null ? "—" : value.toFixed(1));

/** A group's boletas for the term, each one a PDF on demand. */
export function ReportCardsDialog({ period, cycleId, client, onClose, download: save = downloadBlob }: ReportCardsDialogProps): ReactElement {
  const groups = useLevelGroups(cycleId, period.levelId);
  const [groupId, setGroupId] = useState("");
  const cards = useReportCards(client, period.termId, groupId);
  const [downloadError, setDownloadError] = useState<string | undefined>(undefined);

  // A level with a single group needs no choice.
  useEffect(() => {
    if (groups.kind === "ready" && groups.value.length === 1) setGroupId(groups.value[0]?.id ?? "");
  }, [groups]);

  const download = async (card: ReportCard): Promise<void> => {
    setDownloadError(undefined);
    try {
      const file = await client.reportCardPdf(card.id);
      save(file.blob, file.fileName ?? boletaFileName(card.studentName, period.termName, card.version));
    } catch {
      setDownloadError(`No se pudo descargar la boleta de ${card.studentName}.`);
    }
  };

  return (
    <DrawerShell title={`Boletas · ${periodTitle(period)}`} subtitle="Formato interno del colegio · PDF al momento" icon="documents" size="lg" onClose={onClose}>
      <div style={{ display: "grid", gap: 14 }}>
        {groups.kind === "failed" && <ErrorAlert message="No se pudieron cargar los grupos." />}
        {groups.kind === "ready" && (
          <SelectField label="Grupo" value={groupId} options={groups.value.map((g) => ({ value: g.id, label: g.label }))} onChange={setGroupId} />
        )}
        {cards.kind === "loading" && <EmptyHint>Cargando boletas…</EmptyHint>}
        {cards.kind === "failed" && <ErrorAlert message="No se pudieron cargar las boletas." />}
        {cards.kind === "ready" && cards.value.length === 0 && <EmptyHint>Este grupo aún no tiene boletas de este periodo.</EmptyHint>}
        {cards.kind === "ready" && cards.value.length > 0 && (
          <div style={{ overflowX: "auto", contain: "inline-size" }}>
            <table aria-label="Boletas del grupo" style={{ width: "100%", borderCollapse: "collapse" }}>
              <thead>
                <tr>
                  <th scope="col" style={TH}>Alumno</th>
                  <th scope="col" style={TH}>Versión</th>
                  <th scope="col" style={TH}>Promedio general</th>
                  <th scope="col" aria-label="Acciones" style={TH} />
                </tr>
              </thead>
              <tbody>
                {cards.value.map((card) => (
                  <tr key={card.id}>
                    <th scope="row" style={{ ...TD, textAlign: "left", fontWeight: 600 }}>{card.studentName}</th>
                    <td style={TD}>{`v${card.version}`}</td>
                    <td style={{ ...TD, fontFamily: "var(--font-display)", fontWeight: 700 }}>{average(card.generalAverage)}</td>
                    <td style={{ ...TD, textAlign: "right" }}>
                      <button type="button" aria-label={`Descargar PDF de ${card.studentName}`} onClick={() => void download(card)}
                        style={{ display: "inline-flex", alignItems: "center", gap: 6, minHeight: 44, padding: "0 14px", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface-3)", color: "var(--text)", fontWeight: 600, cursor: "pointer", whiteSpace: "nowrap" }}>
                        <Icon name="download" size={16} />
                        Descargar PDF
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {downloadError !== undefined && <ErrorAlert message={downloadError} />}
      </div>
    </DrawerShell>
  );
}

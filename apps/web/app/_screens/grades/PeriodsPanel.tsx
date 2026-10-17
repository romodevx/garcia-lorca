"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { Toast } from "../../_design/Toast";
import { API_URL } from "../../api-url";
import { periodsClient, type GradingPeriod } from "../../grades/periods";
import { EmptyHint, ErrorAlert } from "../config/parts";
import { ClosePeriodDialog } from "./periods/ClosePeriodDialog";
import { publishedMessage } from "./periods/format";
import { PeriodCard, type PeriodAction } from "./periods/PeriodCard";
import { ReopenPeriodDialog } from "./periods/ReopenPeriodDialog";
import { ReportCardsDialog } from "./periods/ReportCardsDialog";
import { usePeriods } from "./periods/usePeriods";

export interface PeriodsPanelProps {
  /** `grades:close` — closing and reopening are offered only with it (UX; the gateway enforces it). */
  canClose: boolean;
}

interface OpenDialog {
  action: PeriodAction;
  period: GradingPeriod;
}

/** Which card takes the focus after its dialog finished, and a counter so the same card can take it twice. */
interface FocusRequest {
  termId: string;
  token: number;
}

const TOAST_MS = 4000;

/** Periods grouped by level, in the order the server lists the levels and by ordinal inside each. */
function byLevel(periods: readonly GradingPeriod[]): { levelId: string; levelName: string; periods: GradingPeriod[] }[] {
  const levels = new Map<string, { levelId: string; levelName: string; periods: GradingPeriod[] }>();
  for (const p of periods) {
    const level = levels.get(p.levelId) ?? { levelId: p.levelId, levelName: p.levelName, periods: [] };
    level.periods.push(p);
    levels.set(p.levelId, level);
  }
  return [...levels.values()].map((l) => ({ ...l, periods: [...l.periods].sort((a, b) => a.ordinal - b.ordinal) }));
}

/**
 * The trimesters of the current cycle, per level: how far capture got, and —
 * for the director — closing them (which issues and publishes the boletas)
 * and reopening them to correct.
 */
export function PeriodsPanel({ canClose }: PeriodsPanelProps): ReactElement {
  const client = useMemo(() => periodsClient(API_URL), []);
  const { load, reload } = usePeriods(client);
  const [dialog, setDialog] = useState<OpenDialog | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const [focus, setFocus] = useState<FocusRequest | undefined>(undefined);

  useEffect(() => {
    if (toast === undefined) return;
    const timer = setTimeout(() => setToast(undefined), TOAST_MS);
    return () => clearTimeout(timer);
  }, [toast]);

  const done = (message: string): void => {
    if (dialog !== undefined) {
      const termId = dialog.period.termId;
      setFocus((prev) => ({ termId, token: (prev?.token ?? 0) + 1 }));
    }
    setDialog(undefined);
    setToast(message);
    reload();
  };
  const dismiss = (): void => setDialog(undefined);

  if (load.kind === "loading") return <EmptyHint>Cargando periodos…</EmptyHint>;
  if (load.kind === "failed") return <ErrorAlert message="No se pudieron cargar los periodos." />;
  if (load.kind === "noCycle") return <EmptyHint>Aún no hay ciclo escolar. Créalo en Configuración.</EmptyHint>;

  return (
    <div style={{ display: "grid", gap: 22 }}>
      {load.periods.length === 0 && <EmptyHint>Este ciclo aún no tiene periodos. Créalos en Configuración → Periodos.</EmptyHint>}
      {byLevel(load.periods).map((level) => (
        <section key={level.levelId} aria-labelledby={`level-${level.levelId}`}>
          <h2 id={`level-${level.levelId}`} style={{ fontSize: 18, fontWeight: 700, color: "var(--text)", marginBottom: 10 }}>{level.levelName}</h2>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(min(100%, 280px), 1fr))", gap: 14 }}>
            {level.periods.map((p) => (
              <PeriodCard
                key={p.termId}
                period={p}
                canClose={canClose}
                focusToken={focus?.termId === p.termId ? focus.token : undefined}
                onAction={(action, period) => setDialog({ action, period })}
              />
            ))}
          </div>
        </section>
      ))}

      {dialog?.action === "close" && (
        <ClosePeriodDialog period={dialog.period} client={client} onClose={dismiss} onClosed={(result) => done(publishedMessage(result))} onStale={done} />
      )}
      {dialog?.action === "reopen" && (
        <ReopenPeriodDialog period={dialog.period} client={client} onClose={dismiss} onReopened={() => done("Periodo reabierto")} onStale={done} />
      )}
      {dialog?.action === "cards" && <ReportCardsDialog period={dialog.period} cycleId={load.cycle.id} client={client} onClose={dismiss} />}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </div>
  );
}

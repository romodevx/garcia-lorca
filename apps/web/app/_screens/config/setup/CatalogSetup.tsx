"use client";

import { useEffect, useRef, useState, type CSSProperties, type ReactElement } from "react";

import { Page } from "../../../_design/Page";
import { Toast } from "../../../_design/Toast";
import type { CatalogProps } from "../Catalog";
import { SetupHeader } from "./SetupHeader";
import { SetupPanes } from "./SetupPanes";
import { SetupPresentationProvider } from "../SetupPresentation";
import { SetupStepList } from "./SetupStepList";
import { COMPACT_MAX_PX, NARROW_MAX_PX, useMaxWidth } from "./layout";
import { deriveSteps, type SetupInput } from "./setupSteps";
import { useCatalogState, type CatalogState } from "../useCatalogState";
import { readStoredStep, useOpenStep } from "./useOpenStep";
import { useSetupActions } from "./useSetupActions";
import { useSetupStatus, type SetupStatus } from "./useSetupStatus";

/** Read by assistive tech, not seen. */
const visuallyHidden: CSSProperties = { position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap" };

function setupInput(state: CatalogState, status: SetupStatus): SetupInput {
  return {
    profile: status.profile,
    cycles: state.cycles.list,
    selectedCycleId: state.cycles.selectedId,
    levels: state.levels.list,
    selectedLevelId: state.levels.selectedId,
    grades: status.grades,
    terms: status.terms,
    groups: status.groups,
    plans: status.plans,
    closedDays: status.closedDays,
  };
}

/**
 * Configuración académica as "Puesta en marcha" (module_config_setup,
 * PLAN-redesign-configuracion): the eight steps on the left, the open one on
 * the right. The same state, panels, calls and arguments as `Catalog` — only
 * order, grouping and hierarchy change. The gateway is the security boundary.
 */
export function CatalogSetup({ onBack, canDeleteLevels = false, onDeleteLevel, notice, onOpenLevelPeriods, onOpenStudentImport, canWriteConfig = false }: CatalogProps): ReactElement {
  const state = useCatalogState(notice, canWriteConfig);
  const status = useSetupStatus(state.cycles.selectedId);
  const actions = useSetupActions(state, status.refresh);
  const [stored] = useState(readStoredStep);
  const view = deriveSteps(setupInput(state, status), stored ?? null);
  const loaded = !state.load.loading && state.load.error === undefined;
  const nav = useOpenStep(view.initialStepId, loaded && view.settled, stored);
  const narrow = useMaxWidth(NARROW_MAX_PX);
  const compact = useMaxWidth(COMPACT_MAX_PX);
  const pricing = state.pricing.active;
  const main = useRef<HTMLDivElement>(null);

  // A step the user opens takes focus at its h2 (§1.7).
  useEffect(() => {
    if (nav.focusRequest === 0 || nav.openId === undefined) return;
    main.current?.querySelector<HTMLElement>(`[data-step="${nav.openId}"] h2`)?.focus();
  }, [nav.focusRequest, nav.openId]);

  return (
    <Page>
      {!pricing && <SetupHeader onBack={onBack} subtitle={loaded ? view.subtitle : undefined} />}
      {state.load.loading && <p className="fade-up" style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando catálogo…</p>}
      {state.load.error !== undefined && <p role="alert" className="fade-up" style={{ color: "var(--bad-fg)", fontSize: 15 }}>{state.load.error}</p>}
      {loaded && (
        <SetupPresentationProvider compact={compact}>
          <div className={pricing ? "setup-layout setup-layout-solo" : "setup-layout"}>
            {!pricing && (
              <SetupStepList steps={view.steps} cardTitle={view.cardTitle} openId={nav.openId} onOpen={nav.open} onOpenStudentImport={onOpenStudentImport} importHint={view.importHint} showTrash={canDeleteLevels} narrow={narrow} />
            )}
            <div ref={main} className="setup-main">
              <SetupPanes
                steps={view.steps}
                openId={nav.openId}
                onOpen={nav.open}
                state={state}
                status={status}
                actions={actions}
                canDeleteLevels={canDeleteLevels}
                canWriteConfig={canWriteConfig}
                onDeleteLevel={onDeleteLevel}
                onOpenLevelPeriods={onOpenLevelPeriods}
              />
            </div>
          </div>
        </SetupPresentationProvider>
      )}
      <p aria-live="polite" style={visuallyHidden}>{nav.announcement}</p>
      {state.notice.shown && notice !== undefined && <Toast>{notice}</Toast>}
    </Page>
  );
}

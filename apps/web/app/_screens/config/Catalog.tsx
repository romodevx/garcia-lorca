"use client";

import type { ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Toast } from "../../_design/Toast";
import { CycleBar } from "./CycleBar";
import { GradeLevelsPanel } from "./GradeLevelsPanel";
import { GroupsPanel } from "./GroupsPanel";
import { LevelsPanel } from "./LevelsPanel";
import { TuitionPanel } from "./TuitionPanel";
import { LevelTrashPanel } from "./levelDeletion/LevelTrashPanel";
import { CalendarPanel } from "./CalendarPanel";
import { SchoolProfilePanel } from "./SchoolProfilePanel";
import { SubjectsPanel } from "./SubjectsPanel";
import { EmptyHint, ErrorAlert } from "./parts";
import { TermsPanel } from "./TermsPanel";
import { LevelPeriodsEntry } from "./periods/LevelPeriodsEntry";
import { StudentImportEntry } from "../studentImport/StudentImportEntry";
import { useCatalogState } from "./useCatalogState";

export interface CatalogProps {
  onBack: () => void;
  /** Owner-only (levels:delete): the delete action on a level and the Papelera. */
  canDeleteLevels?: boolean;
  /** Opens the three-step deletion page of one level. */
  onDeleteLevel?: (levelId: string) => void;
  /** A message another screen left for this one (a finished deletion). */
  notice?: string;
  /** Opens the evaluation periods page of one level, grade by grade (T14). */
  onOpenLevelPeriods?: (levelId: string) => void;
  /**
   * Opens Migrar alumnos (ADR 0033). Given only with module_student_import on
   * and students:import held; absent, neither way in is shown.
   */
  onOpenStudentImport?: () => void;
}

/**
 * Academic-catalog admin surface (module_config). Its data fetching and create
 * handlers live in `useCatalogState`; this view passes plain data + async
 * `onCreate` callbacks down to small presentational panels. Selecting a cycle/level/grade progressively
 * reveals the next tier — fewest clicks, with sensible prefilled defaults. The
 * gateway (behind the flag + config:read/write) is the security boundary; this
 * screen is UX only.
 */
export function Catalog({ onBack, canDeleteLevels = false, onDeleteLevel, notice, onOpenLevelPeriods, onOpenStudentImport }: CatalogProps): ReactElement {
  const { load, cycles, levels, grades, groups, terms: levelTerms, pricing: pricingState, notice: noticeState } = useCatalogState(notice);
  const selectedCycle = cycles.selected;
  const selectedLevel = levels.selected;
  const selectedGradeLevel = grades.selected;

  /*
   * Capturar precios se lleva la pantalla completa: mientras esto es true, el
   * catálogo entero no se dibuja.
   *
   * Se ESCONDE lo de alrededor en vez de devolver otro árbol. El retorno temprano
   * que había aquí ponía a `<TuitionPanel>` en otra posición, así que React lo
   * desmontaba y lo volvía a montar: su lista de planes se reiniciaba a vacía y
   * `TuitionPage` sembraba sus campos desde `undefined`. "Editar" abría el
   * tabulador EN BLANCO y guardar sobrescribía cuotas, cobranza, día límite y
   * fecha de inscripción con los valores por omisión. El panel tiene que quedarse
   * exactamente donde está.
   */
  const pricing = pricingState.active;

  return (
    <Page>
      {!pricing && (
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>
      )}

      {!pricing && (
      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--forest) 38%, transparent)" }}>
          <Icon name="settings" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Configuración académica</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Ciclos, niveles, grados, materias, grupos y periodos</p>
        </div>
        {onOpenStudentImport !== undefined && (
          <div style={{ marginLeft: "auto" }}>
            <Btn kind="ghost" icon="upload" onClick={onOpenStudentImport}>Migrar alumnos</Btn>
          </div>
        )}
      </div>
      )}

      {!pricing && !load.loading && load.error === undefined && (
        <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 24 }}>
          {([["calendar", "lime", "Ciclos", cycles.list.length], ["graduation", "forest", "Niveles", levels.list.length], ["students", "purple", "Grados", grades.list.length]] as const).map(([icon, c, label, val], i) => (
            <div key={i} style={{ flex: "1 1 180px", minWidth: 0, display: "flex", alignItems: "center", gap: 14, padding: "16px 20px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
              <div style={{ width: 44, height: 44, borderRadius: 13, flexShrink: 0, display: "grid", placeItems: "center", background: `color-mix(in oklab, var(--${c}) 16%, var(--surface))`, color: `var(--${c})` }}><Icon name={icon} size={22} /></div>
              <div><div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600 }}>{label}</div><div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 22, color: "var(--text)", lineHeight: 1.1 }}>{val}</div></div>
            </div>
          ))}
        </div>
      )}

      {load.loading ? (
        <p className="fade-up" style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando catálogo…</p>
      ) : load.error !== undefined ? (
        <p role="alert" className="fade-up" style={{ color: "var(--bad-fg)", fontSize: 15 }}>{load.error}</p>
      ) : (
        <div className="fade-up" style={{ display: "grid", gap: 18 }}>
          {!pricing && <SchoolProfilePanel />}
          {!pricing && (
            <CycleBar cycles={cycles.list} selectedCycleId={cycles.selectedId} onSelect={cycles.select} onCreate={cycles.create} onActivate={cycles.activate} />
          )}
          {!pricing && onOpenStudentImport !== undefined && <StudentImportEntry onOpen={onOpenStudentImport} />}

          {/*
            The money screen for the academic catalogue. It reads and writes
            through finance — the ledger owns prices — but it belongs in front of
            a director's eyes here, next to the levels it prices.
          */}
          {selectedCycle !== undefined && (
            <TuitionPanel
              cycle={selectedCycle}
              levels={levels.list}
              previousCycle={cycles.previous}
              pricingLevelId={pricingState.levelId}
              onPrice={pricingState.setLevelId}
            />
          )}
          {!pricing && selectedCycle !== undefined && <CalendarPanel cycle={selectedCycle} />}

          {!pricing && (
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(min(340px, 100%), 1fr))", gap: 18 }}>
            <LevelsPanel
              levels={levels.list}
              selectedLevelId={levels.selectedId}
              onSelect={levels.select}
              onCreate={levels.create}
              onSetAttendanceMode={levels.setAttendanceMode}
              onSetGrading={levels.setGrading}
              onDelete={canDeleteLevels ? onDeleteLevel : undefined}
            />

            {selectedLevel !== undefined && (
              <GradeLevelsPanel
                key={selectedLevel.id}
                level={selectedLevel}
                gradeLevels={grades.list}
                selectedGradeLevelId={grades.selectedId}
                onSelect={grades.select}
                onCreate={grades.create}
              />
            )}

            {selectedLevel !== undefined && selectedCycle !== undefined && (
              // Full row: the months grid needs the width, and scrolls inside its own box when there is none.
              <div style={{ gridColumn: "1 / -1", minWidth: 0 }}>
                {levelTerms.load.kind === "ready" && (
                  <TermsPanel
                    key={`${selectedCycle.id}:${selectedLevel.id}`}
                    cycle={selectedCycle}
                    level={selectedLevel}
                    levels={levels.list}
                    terms={levelTerms.load.terms}
                    onCreate={levelTerms.create}
                    onSetMonths={levelTerms.setMonths}
                    onCopyMonths={levelTerms.copyMonths}
                  />
                )}
                {levelTerms.load.kind === "loading" && <EmptyHint>Cargando periodos…</EmptyHint>}
                {levelTerms.load.kind === "failed" && <ErrorAlert message="No se pudieron cargar los periodos de este nivel." />}
              </div>
            )}

            {selectedLevel !== undefined && onOpenLevelPeriods !== undefined && (
              // Full row: the grades' own evaluation periods live on their own page (T14).
              <div style={{ gridColumn: "1 / -1", minWidth: 0 }}>
                <LevelPeriodsEntry level={selectedLevel} onOpen={onOpenLevelPeriods} />
              </div>
            )}

            {selectedLevel !== undefined && (
              // Full row: the level's subjects, grade by grade (T13).
              <div style={{ gridColumn: "1 / -1", minWidth: 0 }}>
                <SubjectsPanel
                  key={`s:${selectedLevel.id}`}
                  level={selectedLevel}
                  gradeLevels={grades.list.filter((g) => g.levelId === selectedLevel.id)}
                  selectedGradeLevelId={grades.selectedId}
                />
              </div>
            )}

            {selectedGradeLevel !== undefined && selectedCycle !== undefined && (
              <GroupsPanel
                key={`${selectedCycle.id}:${selectedGradeLevel.id}`}
                cycle={selectedCycle}
                gradeLevel={selectedGradeLevel}
                groups={groups.list}
                onCreate={groups.create}
              />
            )}
          </div>
          )}

          {!pricing && canDeleteLevels && <LevelTrashPanel onRestored={levels.reload} />}
        </div>
      )}
      {noticeState.shown && notice !== undefined && <Toast>{notice}</Toast>}
    </Page>
  );
}

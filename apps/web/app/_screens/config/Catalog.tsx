"use client";

import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import type { AttendanceMode } from "../../attendance/sheet";
import {
  createCycle,
  createGradeLevel,
  createGroup,
  createLevel,
  updateLevelAttendanceMode,
  updateLevelGrading,
  createSubject,
  activateCycle,
  listCycles,
  listGradeLevels,
  listGroups,
  listLevels,
  listSubjects,
  copyTermMonths,
  type CopyTermMonthsInput,
  type CreateCycleInput,
  type CreateGradeLevelInput,
  type CreateGroupInput,
  type CreateLevelInput,
  type CreateSubjectInput,
  type Cycle,
  type GradeLevel,
  type Group,
  type Level,
  type LevelGrading,
  type Subject,
  type Term,
} from "../../config/api";
import { CycleBar } from "./CycleBar";
import { GradeLevelsPanel } from "./GradeLevelsPanel";
import { GroupsPanel } from "./GroupsPanel";
import { LevelsPanel } from "./LevelsPanel";
import { TuitionPanel } from "./TuitionPanel";
import { CalendarPanel } from "./CalendarPanel";
import { SchoolProfilePanel } from "./SchoolProfilePanel";
import { SubjectsPanel } from "./SubjectsPanel";
import { EmptyHint, ErrorAlert } from "./parts";
import { TermsPanel } from "./TermsPanel";
import { useLevelTerms } from "./useLevelTerms";

import { API_URL } from "../../api-url";

// Gateway base URL for the browser (public env; matches AppShell/compose).

export interface CatalogProps {
  onBack: () => void;
}

/**
 * Academic-catalog admin surface (module_config). Centralizes all data fetching
 * and the create handlers, passing plain data + async `onCreate` callbacks down
 * to small presentational panels. Selecting a cycle/level/grade progressively
 * reveals the next tier — fewest clicks, with sensible prefilled defaults. The
 * gateway (behind the flag + config:read/write) is the security boundary; this
 * screen is UX only.
 */
export function Catalog({ onBack }: CatalogProps): ReactElement {
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [levels, setLevels] = useState<Level[]>([]);
  const [gradeLevels, setGradeLevels] = useState<GradeLevel[]>([]);
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);

  const [selectedCycleId, setSelectedCycleId] = useState<string | undefined>(undefined);
  const [selectedLevelId, setSelectedLevelId] = useState<string | undefined>(undefined);
  const [selectedGradeLevelId, setSelectedGradeLevelId] = useState<string | undefined>(undefined);

  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);

  // Boot: load the cycles + the levels catalog, auto-selecting a cycle.
  useEffect(() => {
    let active = true;
    setLoading(true);
    Promise.all([listCycles(API_URL), listLevels(API_URL)])
      .then(([cs, ls]) => {
        if (!active) return;
        setCycles(cs);
        setLevels(ls);
        const preferred = cs.find((c) => c.status === "active") ?? cs[0];
        if (preferred !== undefined) setSelectedCycleId((prev) => prev ?? preferred.id);
      })
      .catch(() => active && setLoadError("No se pudo cargar el catálogo."))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  // Selecting a level loads its grades and clears any grade selection.
  useEffect(() => {
    if (selectedLevelId === undefined) return;
    setSelectedGradeLevelId(undefined);
    let active = true;
    listGradeLevels(API_URL, selectedLevelId)
      .then((gs) => active && setGradeLevels(gs))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [selectedLevelId]);

  // Selecting a grade loads its subjects.
  useEffect(() => {
    if (selectedGradeLevelId === undefined) return;
    let active = true;
    listSubjects(API_URL, selectedGradeLevelId)
      .then((ss) => active && setSubjects(ss))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [selectedGradeLevelId]);

  // Groups are per (cycle, grade level).
  useEffect(() => {
    if (selectedCycleId === undefined || selectedGradeLevelId === undefined) return;
    let active = true;
    listGroups(API_URL, selectedCycleId, selectedGradeLevelId)
      .then((gs) => active && setGroups(gs))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [selectedCycleId, selectedGradeLevelId]);

  // Terms are per (cycle, level); the hook never hands over another selection's list.
  const levelTerms = useLevelTerms(selectedCycleId, selectedLevelId);

  const handleActivateCycle = useCallback(async (id: string): Promise<void> => {
    const activated = await activateCycle(API_URL, id);
    // Every other cycle is stood down server-side, so the local list mirrors that
    // rather than showing two live years until the next reload.
    setCycles((prev) => prev.map((c) => (c.id === activated.id ? activated : c.status === "active" ? { ...c, status: "draft" } : c)));
  }, []);

  const handleCreateCycle = useCallback(async (input: CreateCycleInput): Promise<void> => {
    const created = await createCycle(API_URL, input);
    setCycles((prev) => [...prev, created]);
    setSelectedCycleId(created.id);
  }, []);

  const handleCreateLevel = useCallback(async (input: CreateLevelInput): Promise<void> => {
    const created = await createLevel(API_URL, input);
    setLevels((prev) => [...prev, created]);
    setSelectedLevelId(created.id);
  }, []);

  const handleSetAttendanceMode = useCallback(async (levelId: string, mode: AttendanceMode): Promise<void> => {
    const updated = await updateLevelAttendanceMode(API_URL, levelId, mode);
    setLevels((prev) => prev.map((l) => (l.id === levelId ? { ...l, ...updated, attendanceMode: updated.attendanceMode ?? mode } : l)));
  }, []);

  const handleSetGrading = useCallback(async (levelId: string, grading: LevelGrading): Promise<void> => {
    const updated = await updateLevelGrading(API_URL, levelId, grading);
    setLevels((prev) => prev.map((l) => (l.id === levelId ? { ...l, ...updated, grading: updated.grading ?? grading } : l)));
  }, []);

  const handleCreateGradeLevel = useCallback(async (input: CreateGradeLevelInput): Promise<void> => {
    const created = await createGradeLevel(API_URL, input);
    setGradeLevels((prev) => [...prev, created]);
  }, []);

  const handleCreateSubject = useCallback(async (input: CreateSubjectInput): Promise<void> => {
    const created = await createSubject(API_URL, input);
    setSubjects((prev) => [...prev, created]);
  }, []);

  const handleCreateGroup = useCallback(async (input: CreateGroupInput): Promise<void> => {
    const created = await createGroup(API_URL, input);
    setGroups((prev) => [...prev, created]);
  }, []);

  const handleCopyTermMonths = useCallback((input: CopyTermMonthsInput): Promise<Term[]> => copyTermMonths(API_URL, input), []);


  const selectedCycle = cycles.find((c) => c.id === selectedCycleId);

  /**
   * El ciclo inmediatamente anterior al seleccionado, que es de donde el
   * tabulador ofrece copiar.
   *
   * Se elige por FECHA DE INICIO, no por el orden en que llegó la lista ni por
   * el nombre: "2025-2026" y "2026-2027" ordenan bien como texto por casualidad,
   * y un ciclo llamado "Verano" rompería esa casualidad sin avisar.
   */
  const previousCycle = useMemo(() => {
    if (selectedCycle === undefined) return undefined;
    return cycles
      .filter((c) => c.startsOn < selectedCycle.startsOn)
      .sort((a, b) => b.startsOn.localeCompare(a.startsOn))[0];
  }, [cycles, selectedCycle]);
  /**
   * El nivel cuyo tabulador se está capturando.
   *
   * Vive aquí y no dentro de `TuitionPanel` porque capturar precios OCUPA la
   * pantalla: mientras esto tiene valor, el catálogo entero no se dibuja. Con el
   * estado adentro, la página salía entre la barra de ciclos y el calendario, y
   * los chips de ciclo seguían clicables — cambiar de año a media captura
   * re-apuntaba el guardado sin limpiar lo escrito.
   */
  const [pricingLevelId, setPricingLevelId] = useState<string | undefined>(undefined);
  const selectedLevel = levels.find((l) => l.id === selectedLevelId);
  const selectedGradeLevel = gradeLevels.find((g) => g.id === selectedGradeLevelId);

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
  const pricing = pricingLevelId !== undefined && selectedCycle !== undefined;

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
      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--forest) 38%, transparent)" }}>
          <Icon name="settings" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Configuración académica</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Ciclos, niveles, grados, materias, grupos y periodos</p>
        </div>
      </div>
      )}

      {!pricing && !loading && loadError === undefined && (
        <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 24 }}>
          {([["calendar", "lime", "Ciclos", cycles.length], ["graduation", "forest", "Niveles", levels.length], ["students", "purple", "Grados", gradeLevels.length]] as const).map(([icon, c, label, val], i) => (
            <div key={i} style={{ flex: "1 1 180px", minWidth: 0, display: "flex", alignItems: "center", gap: 14, padding: "16px 20px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
              <div style={{ width: 44, height: 44, borderRadius: 13, flexShrink: 0, display: "grid", placeItems: "center", background: `color-mix(in oklab, var(--${c}) 16%, var(--surface))`, color: `var(--${c})` }}><Icon name={icon} size={22} /></div>
              <div><div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600 }}>{label}</div><div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 22, color: "var(--text)", lineHeight: 1.1 }}>{val}</div></div>
            </div>
          ))}
        </div>
      )}

      {loading ? (
        <p className="fade-up" style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando catálogo…</p>
      ) : loadError !== undefined ? (
        <p role="alert" className="fade-up" style={{ color: "var(--bad-fg)", fontSize: 15 }}>{loadError}</p>
      ) : (
        <div className="fade-up" style={{ display: "grid", gap: 18 }}>
          {!pricing && <SchoolProfilePanel />}
          {!pricing && (
            <CycleBar cycles={cycles} selectedCycleId={selectedCycleId} onSelect={setSelectedCycleId} onCreate={handleCreateCycle} onActivate={handleActivateCycle} />
          )}

          {/*
            The money screen for the academic catalogue. It reads and writes
            through finance — the ledger owns prices — but it belongs in front of
            a director's eyes here, next to the levels it prices.
          */}
          {selectedCycle !== undefined && (
            <TuitionPanel
              cycle={selectedCycle}
              levels={levels}
              previousCycle={previousCycle}
              pricingLevelId={pricingLevelId}
              onPrice={setPricingLevelId}
            />
          )}
          {!pricing && selectedCycle !== undefined && <CalendarPanel cycle={selectedCycle} />}

          {!pricing && (
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(340px, 1fr))", gap: 18 }}>
            <LevelsPanel levels={levels} selectedLevelId={selectedLevelId} onSelect={setSelectedLevelId} onCreate={handleCreateLevel} onSetAttendanceMode={handleSetAttendanceMode} onSetGrading={handleSetGrading} />

            {selectedLevel !== undefined && (
              <GradeLevelsPanel
                key={selectedLevel.id}
                level={selectedLevel}
                gradeLevels={gradeLevels}
                selectedGradeLevelId={selectedGradeLevelId}
                onSelect={setSelectedGradeLevelId}
                onCreate={handleCreateGradeLevel}
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
                    levels={levels}
                    terms={levelTerms.load.terms}
                    onCreate={levelTerms.create}
                    onSetMonths={levelTerms.setMonths}
                    onCopyMonths={handleCopyTermMonths}
                  />
                )}
                {levelTerms.load.kind === "loading" && <EmptyHint>Cargando periodos…</EmptyHint>}
                {levelTerms.load.kind === "failed" && <ErrorAlert message="No se pudieron cargar los periodos de este nivel." />}
              </div>
            )}

            {selectedGradeLevel !== undefined && (
              <SubjectsPanel key={`s:${selectedGradeLevel.id}`} gradeLevel={selectedGradeLevel} subjects={subjects} onCreate={handleCreateSubject} />
            )}

            {selectedGradeLevel !== undefined && selectedCycle !== undefined && (
              <GroupsPanel
                key={`${selectedCycle.id}:${selectedGradeLevel.id}`}
                cycle={selectedCycle}
                gradeLevel={selectedGradeLevel}
                groups={groups}
                onCreate={handleCreateGroup}
              />
            )}
          </div>
          )}
        </div>
      )}
    </Page>
  );
}

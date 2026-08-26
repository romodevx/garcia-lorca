"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import {
  createCycle,
  createGradeLevel,
  createGroup,
  createLevel,
  createSubject,
  createTerm,
  listCycles,
  listGradeLevels,
  listGroups,
  listLevels,
  listSubjects,
  listTerms,
  type CreateCycleInput,
  type CreateGradeLevelInput,
  type CreateGroupInput,
  type CreateLevelInput,
  type CreateSubjectInput,
  type CreateTermInput,
  type Cycle,
  type GradeLevel,
  type Group,
  type Level,
  type Subject,
  type Term,
} from "../../config/api";
import { CycleBar } from "./CycleBar";
import { GradeLevelsPanel } from "./GradeLevelsPanel";
import { GroupsPanel } from "./GroupsPanel";
import { LevelsPanel } from "./LevelsPanel";
import { SubjectsPanel } from "./SubjectsPanel";
import { TermsPanel } from "./TermsPanel";

// Gateway base URL for the browser (public env; matches AppShell/compose).
const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

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
  const [terms, setTerms] = useState<Term[]>([]);

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

  // Terms are per (cycle, level).
  useEffect(() => {
    if (selectedCycleId === undefined || selectedLevelId === undefined) return;
    let active = true;
    listTerms(API_URL, selectedCycleId, selectedLevelId)
      .then((ts) => active && setTerms(ts))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [selectedCycleId, selectedLevelId]);

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

  const handleCreateTerm = useCallback(async (input: CreateTermInput): Promise<void> => {
    const created = await createTerm(API_URL, input);
    setTerms((prev) => [...prev, created]);
  }, []);


  const selectedCycle = cycles.find((c) => c.id === selectedCycleId);
  const selectedLevel = levels.find((l) => l.id === selectedLevelId);
  const selectedGradeLevel = gradeLevels.find((g) => g.id === selectedGradeLevelId);

  return (
    <div style={{ maxWidth: 1320, margin: "0 auto", padding: "24px 28px 64px" }}>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--forest) 38%, transparent)" }}>
          <Icon name="settings" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Configuración académica</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Ciclos, niveles, grados, materias, grupos y periodos</p>
        </div>
      </div>

      {!loading && loadError === undefined && (
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
          <CycleBar cycles={cycles} selectedCycleId={selectedCycleId} onSelect={setSelectedCycleId} onCreate={handleCreateCycle} />

          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(340px, 1fr))", gap: 18 }}>
            <LevelsPanel levels={levels} selectedLevelId={selectedLevelId} onSelect={setSelectedLevelId} onCreate={handleCreateLevel} />

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
              <TermsPanel key={`${selectedCycle.id}:${selectedLevel.id}`} cycle={selectedCycle} level={selectedLevel} terms={terms} onCreate={handleCreateTerm} />
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
        </div>
      )}
    </div>
  );
}

"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { API_URL } from "../../../api-url";
import type { AttendanceMode } from "../../../attendance/sheet";
import {
  activateCycle,
  copyTermMonths,
  createCycle,
  createGradeLevel,
  createGroup,
  createLevel,
  listCycles,
  listGradeLevels,
  listGroups,
  listLevels,
  updateLevelAttendanceMode,
  updateLevelGrading,
  type CopyTermMonthsInput,
  type CreateCycleInput,
  type CreateGradeLevelInput,
  type CreateGroupInput,
  type CreateLevelInput,
  type Cycle,
  type GradeLevel,
  type Group,
  type Level,
  type LevelGrading,
  type Term,
} from "../../../config/api";
import { useLevelTerms, type LevelTerms } from "../useLevelTerms";

/** How long a notice left by another screen stays up. */
const NOTICE_MS = 8_000;

export interface CyclesSlice {
  list: Cycle[];
  selectedId: string | undefined;
  selected: Cycle | undefined;
  /** The cycle right before the selected one, by start date: where the tabulador copies from. */
  previous: Cycle | undefined;
  select: (id: string) => void;
  create: (input: CreateCycleInput) => Promise<void>;
  activate: (id: string) => Promise<void>;
}

export interface LevelsSlice {
  list: Level[];
  selectedId: string | undefined;
  selected: Level | undefined;
  select: (id: string) => void;
  create: (input: CreateLevelInput) => Promise<void>;
  setAttendanceMode: (levelId: string, mode: AttendanceMode) => Promise<void>;
  setGrading: (levelId: string, grading: LevelGrading) => Promise<void>;
  /** A level restored from the Papelera is back: re-read the list it belongs to. */
  reload: () => void;
}

export interface GradesSlice {
  /** The grades of the selected level only. */
  list: GradeLevel[];
  selectedId: string | undefined;
  selected: GradeLevel | undefined;
  select: (id: string) => void;
  create: (input: CreateGradeLevelInput) => Promise<void>;
}

export interface GroupsSlice {
  /** The groups of the selected (cycle, grade) only. */
  list: Group[];
  create: (input: CreateGroupInput) => Promise<void>;
}

export interface TermsSlice extends LevelTerms {
  copyMonths: (input: CopyTermMonthsInput) => Promise<Term[]>;
}

export interface PricingSlice {
  /** The level whose tabulador is being captured. */
  levelId: string | undefined;
  setLevelId: (id: string | undefined) => void;
  /** Capturing prices takes the whole screen. */
  active: boolean;
}

export interface LoadSlice {
  loading: boolean;
  error: string | undefined;
}

export interface NoticeSlice {
  shown: boolean;
}

export interface CatalogState {
  load: LoadSlice;
  cycles: CyclesSlice;
  levels: LevelsSlice;
  grades: GradesSlice;
  groups: GroupsSlice;
  terms: TermsSlice;
  pricing: PricingSlice;
  notice: NoticeSlice;
}

/**
 * All of the academic catalog's data fetching, selection and create handlers,
 * shared by every view of the screen. Each slice carries plain data plus the
 * async callbacks the presentational panels receive. The gateway (behind the
 * flag + config:read/write) is the security boundary; this is UX only.
 */
export function useCatalogState(notice: string | undefined): CatalogState {
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [levels, setLevels] = useState<Level[]>([]);
  const [gradeLevels, setGradeLevels] = useState<GradeLevel[]>([]);
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

  const handleCreateGroup = useCallback(async (input: CreateGroupInput): Promise<void> => {
    const created = await createGroup(API_URL, input);
    setGroups((prev) => [...prev, created]);
  }, []);

  // The notice (≥ 8 s, like an undo toast) leaves on its own instead of pinning over the page.
  const [noticeShown, setNoticeShown] = useState(notice !== undefined);
  useEffect(() => {
    if (notice === undefined) return;
    setNoticeShown(true);
    const timer = setTimeout(() => setNoticeShown(false), NOTICE_MS);
    return () => clearTimeout(timer);
  }, [notice]);

  const reloadLevels = useCallback((): void => {
    listLevels(API_URL).then(setLevels).catch(() => {});
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

  return {
    load: { loading, error: loadError },
    cycles: {
      list: cycles,
      selectedId: selectedCycleId,
      selected: selectedCycle,
      previous: previousCycle,
      select: setSelectedCycleId,
      create: handleCreateCycle,
      activate: handleActivateCycle,
    },
    levels: {
      list: levels,
      selectedId: selectedLevelId,
      selected: selectedLevel,
      select: setSelectedLevelId,
      create: handleCreateLevel,
      setAttendanceMode: handleSetAttendanceMode,
      setGrading: handleSetGrading,
      reload: reloadLevels,
    },
    grades: {
      list: gradeLevels,
      selectedId: selectedGradeLevelId,
      selected: selectedGradeLevel,
      select: setSelectedGradeLevelId,
      create: handleCreateGradeLevel,
    },
    groups: { list: groups, create: handleCreateGroup },
    terms: { ...levelTerms, copyMonths: handleCopyTermMonths },
    pricing: {
      levelId: pricingLevelId,
      setLevelId: setPricingLevelId,
      active: pricingLevelId !== undefined && selectedCycle !== undefined,
    },
    notice: { shown: noticeShown },
  };
}

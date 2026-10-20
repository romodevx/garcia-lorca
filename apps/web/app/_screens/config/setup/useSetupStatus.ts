"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { API_URL } from "../../../api-url";
import { listGradeLevels, listGroups, listSubjects, listTerms, type GradeLevel, type Group, type SchoolProfile, type Subject, type Term } from "../../../config/api";
import type { TuitionPlan } from "../../../finance/api";
import { useLatest } from "../useLatest";
import type { StatusRead } from "./setupSteps";

/** What a create invalidates (plan §4.2): one read, or all three on a cycle created or activated. */
export type RefreshKind = "grades" | "terms" | "groups" | "subjects" | "all";


export interface SetupStatus {
  grades: StatusRead<GradeLevel[]>;
  terms: StatusRead<Term[]>;
  groups: StatusRead<Group[]>;
  /** Every subject of the school: subjects belong to a grade, not to a cycle. */
  subjects: StatusRead<Subject[]>;
  profile: StatusRead<SchoolProfile>;
  plans: StatusRead<TuitionPlan[]>;
  closedDays: StatusRead<number>;
  refresh: (kind: RefreshKind) => void;
  /**
   * Apply a write the server already took to a ready read, at once — the
   * refresh that follows confirms it; a read not yet ready is left alone. The
   * «Estructura de la escuela» tree paints from these (PLAN-redesign-estructura H1).
   */
  patchGrades: (change: (list: GradeLevel[]) => GradeLevel[]) => void;
  patchGroups: (change: (list: Group[]) => Group[]) => void;
  /** For `SchoolProfilePanel onLoaded`. */
  onProfileLoaded: (profile: SchoolProfile | "failed") => void;
  /** For `TuitionPanel onPlansLoaded`. */
  onPlansLoaded: (cycleId: string, plans: TuitionPlan[] | "failed") => void;
  /** For `CalendarPanel onLoaded`. */
  onCalendarLoaded: (cycleId: string, closedDays: number | "failed") => void;
}

const LOADING = { kind: "loading" } as const;
const FAILED = { kind: "failed" } as const;

/** The grades are not per cycle: one key for the whole school. */
const SCHOOL_KEY = "school";

/** A read and the key (cycle, or the school) it belongs to: it never outlives that key. */
interface Keyed<T> {
  key: string;
  read: StatusRead<T>;
}

const toRead = <T,>(result: T | "failed"): StatusRead<T> => (result === "failed" ? FAILED : { kind: "ready", value: result });

/** The stored read when it belongs to `key`; another key's answer is "still loading". */
function readFor<T>(stored: Keyed<T> | undefined, key: string | undefined): StatusRead<T> {
  return key !== undefined && stored?.key === key ? stored.read : LOADING;
}

/**
 * One keyed read with a refresh. Only the newest request may answer: a late
 * answer for an older key, or for an older refresh of the same key, is dropped.
 * While a refresh is in flight the previous answer stays (no flicker to
 * "loading"), and a refresh that FAILS keeps it too: the tree that paints from
 * it must not empty itself over one lost request (H2). Only a first read fails.
 */
function useKeyedRead<T>(key: string | undefined, load: (key: string) => Promise<T>): { read: StatusRead<T>; refresh: () => void; patch: (change: (value: T) => T) => void } {
  const [stored, setStored] = useState<Keyed<T> | undefined>(undefined);
  const newest = useRef(0);

  const fetchFor = useCallback(
    (k: string): void => {
      const ticket = ++newest.current;
      const settle = (read: StatusRead<T>): void => {
        if (ticket !== newest.current) return;
        setStored((prev) =>
          read.kind === "failed" && prev?.key === k && prev.read.kind === "ready"
            ? { key: k, read: { kind: "ready", value: prev.read.value, stale: true } }
            : { key: k, read },
        );
      };
      load(k)
        .then((value) => settle({ kind: "ready", value }))
        .catch(() => settle(FAILED));
    },
    [load],
  );

  const patch = useCallback((change: (value: T) => T): void => {
    setStored((prev) => (prev?.read.kind === "ready" ? { key: prev.key, read: { ...prev.read, value: change(prev.read.value) } } : prev));
  }, []);

  useEffect(() => {
    if (key !== undefined) fetchFor(key);
  }, [key, fetchFor]);

  const refresh = useCallback((): void => {
    if (key !== undefined) fetchFor(key);
  }, [key, fetchFor]);

  return { read: readFor(stored, key), refresh, patch };
}

const loadAllGrades = (): Promise<GradeLevel[]> => listGradeLevels(API_URL);
const loadCycleTerms = (cycleId: string): Promise<Term[]> => listTerms(API_URL, cycleId);
const loadCycleGroups = (cycleId: string): Promise<Group[]> => listGroups(API_URL, cycleId);
const loadAllSubjects = (): Promise<Subject[]> => listSubjects(API_URL, "");

/** A value a mounted panel reports for one cycle; another cycle's report is dropped. */
function useCycleReport<T>(cycleId: string | undefined): { read: StatusRead<T>; report: (cycleId: string, result: T | "failed") => void } {
  const [stored, setStored] = useState<Keyed<T> | undefined>(undefined);
  const current = useLatest(cycleId);
  const report = useCallback(
    (from: string, result: T | "failed"): void => {
      if (from === current.current) setStored({ key: from, read: toRead(result) });
    },
    [current],
  );
  return { read: readFor(stored, cycleId), report };
}

/**
 * Everything the setup view's step list needs to mark its steps for the
 * selected cycle (plan §4.1–§4.2). Three unfiltered reads of its own — every
 * grade, and every term and group of the cycle — plus what the mounted panels
 * already read and report (profile, tabuladores, closed days), so nothing is
 * read twice. Used only by the setup view: the old view never calls it.
 */
export function useSetupStatus(cycleId: string | undefined): SetupStatus {
  const grades = useKeyedRead(SCHOOL_KEY, loadAllGrades);
  const terms = useKeyedRead(cycleId, loadCycleTerms);
  const groups = useKeyedRead(cycleId, loadCycleGroups);
  const subjects = useKeyedRead(SCHOOL_KEY, loadAllSubjects);

  const [profile, setProfile] = useState<StatusRead<SchoolProfile>>(LOADING);
  const onProfileLoaded = useCallback((result: SchoolProfile | "failed"): void => setProfile(toRead(result)), []);
  const plans = useCycleReport<TuitionPlan[]>(cycleId);
  const closedDays = useCycleReport<number>(cycleId);

  const { refresh: refreshGrades } = grades;
  const { refresh: refreshTerms } = terms;
  const { refresh: refreshGroups } = groups;
  const { refresh: refreshSubjects } = subjects;
  const refresh = useCallback(
    (kind: RefreshKind): void => {
      if (kind === "grades" || kind === "all") refreshGrades();
      if (kind === "terms" || kind === "all") refreshTerms();
      if (kind === "groups" || kind === "all") refreshGroups();
      if (kind === "subjects" || kind === "all") refreshSubjects();
    },
    [refreshGrades, refreshTerms, refreshGroups, refreshSubjects],
  );

  return {
    patchGrades: grades.patch,
    patchGroups: groups.patch,
    grades: grades.read,
    terms: terms.read,
    groups: groups.read,
    subjects: subjects.read,
    profile,
    plans: plans.read,
    closedDays: closedDays.read,
    refresh,
    onProfileLoaded,
    onPlansLoaded: plans.report,
    onCalendarLoaded: closedDays.report,
  };
}

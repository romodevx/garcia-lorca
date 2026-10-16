"use client";

import { useEffect, useRef, useState } from "react";

import { focusFieldLabelled } from "../../_design/form";
import type { LabelledGroup, Level } from "../../config/api";
import { schoolISODate } from "../../_lib/schoolDay";
import { todayISO } from "../attendance/sheetModel";
import { altaFailureMessage, createdMessage, missingFields, PlacementFailed, saveAlta, type AltaRequest } from "./altaModel";
import { entryDayToSend, FUTURE_ENTRY_ERROR } from "./AltaPlacementFields";

/** Everything the alta page captures. */
export interface AltaDraft {
  curp: string;
  lastName: string;
  maternalLastName: string;
  firstName: string;
  guardianName: string;
  birthDate: string;
  note: string;
  levelId: string;
  gradeLevelId: string;
  /** "" lets capacity choose the group. */
  groupId: string;
  entryDay: string;
}

/** Who the pupil is: what "Crear y agregar otro" clears and a retry cannot change. */
const IDENTITY_BLANK = { curp: "", lastName: "", maternalLastName: "", firstName: "", guardianName: "", birthDate: "", note: "" } as const;

/** The visible label of each field: the summary links by it, in reading order. */
export const FIELD_LABEL: Readonly<Record<string, string>> = {
  curp: "CURP",
  lastName: "Apellido paterno",
  firstName: "Nombre(s)",
  guardianName: "Tutor",
  birthDate: "Fecha de nacimiento",
  levelId: "Nivel",
  gradeLevelId: "Grado",
  enrolledOn: "Fecha de ingreso",
};
const FIELD_ORDER = Object.keys(FIELD_LABEL);
const FIRST_FIELD = "CURP";

export type SaveMode = "close" | "another";

interface Options {
  levels: readonly Level[];
  groups: readonly LabelledGroup[];
  withNote: boolean;
  onCreated: (message: string) => void;
}

/** What blocks the save, keyed by field and in the page's reading order. */
function validate(draft: AltaDraft, today: string): Record<string, string> {
  const found: Record<string, string> = missingFields(draft);
  // The calendar's max binds only the picker; a typed year can still land after today.
  if (draft.birthDate !== "" && draft.birthDate > todayISO()) found.birthDate = "La fecha de nacimiento no puede ser posterior a hoy.";
  if (draft.entryDay > today) found.enrolledOn = FUTURE_ENTRY_ERROR;
  // No level yet: the grade select is still off, so the message goes on Nivel.
  if (found.gradeLevelId !== undefined && draft.levelId === "") {
    found.levelId = found.gradeLevelId;
    delete found.gradeLevelId;
  }
  return Object.fromEntries(FIELD_ORDER.filter((k) => found[k] !== undefined).map((k) => [k, found[k] as string]));
}

export function useAltaForm({ levels, groups, withNote, onCreated }: Options) {
  const [today] = useState(() => schoolISODate());
  const [draft, setDraft] = useState<AltaDraft>(() => ({ ...IDENTITY_BLANK, levelId: "", gradeLevelId: "", groupId: "", entryDay: today }));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [attempt, setAttempt] = useState(0);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [done, setDone] = useState<string | undefined>(undefined);
  const [showMore, setShowMore] = useState(false);
  // Set once the pupil exists and only the placement failed: the retry must
  // not create them again, and who they are can no longer change here.
  const [createdId, setCreatedId] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const [fresh, setFresh] = useState(0);

  useEffect(() => { if (fresh > 0) focusFieldLabelled(FIRST_FIELD); }, [fresh]);

  const set = <K extends keyof AltaDraft>(key: K, value: AltaDraft[K]): void => setDraft((d) => ({ ...d, [key]: value }));
  const clearError = (key: string): void =>
    setErrors((prev) => {
      if (prev[key] === undefined) return prev;
      const rest = { ...prev };
      delete rest[key];
      return rest;
    });
  // A new level or grade sends the group back to Automático.
  const chooseLevel = (levelId: string): void => setDraft((d) => ({ ...d, levelId, gradeLevelId: "", groupId: "" }));
  const chooseGrade = (gradeLevelId: string): void => setDraft((d) => ({ ...d, gradeLevelId, groupId: "" }));

  const request = (level: Level): AltaRequest => ({
    student: {
      curp: draft.curp.trim().toUpperCase(),
      firstName: draft.firstName.trim(),
      lastName: draft.lastName.trim(),
      maternalLastName: draft.maternalLastName.trim(),
      // The level travels as the config level's ID; the server supplies its name (ADR 0014).
      levelId: level.id,
      // Empty on purpose: the server writes it when the pupil is placed.
      gradeGroup: "",
      guardianName: draft.guardianName.trim(),
      ...(draft.birthDate !== "" ? { birthDate: draft.birthDate } : {}),
    },
    gradeLevelId: draft.gradeLevelId,
    groupId: draft.groupId,
    groupLabel: groups.find((g) => g.id === draft.groupId)?.label ?? "",
    enrolledOn: entryDayToSend(draft.entryDay, today),
    note: withNote ? draft.note.trim() : "",
    createdStudentId: createdId,
  });

  const startNext = (message: string): void => {
    setDraft((d) => ({ ...d, ...IDENTITY_BLANK }));
    setCreatedId(undefined);
    setErrors({});
    setDone(message);
    setFresh((n) => n + 1);
  };

  const submit = async (mode: SaveMode): Promise<void> => {
    // Never disabled (it would also break Enter): a press while saving is ignored.
    if (savingRef.current) return;
    const found = validate(draft, today);
    setErrors(found);
    setDone(undefined);
    if (found.birthDate !== undefined) setShowMore(true);
    if (Object.keys(found).length > 0) {
      setAttempt((n) => n + 1);
      return;
    }
    const level = levels.find((l) => l.id === draft.levelId);
    if (level === undefined) {
      // The level vanished from the catalog between choosing and saving.
      setFailure("Elige el nivel y el grado al que entra.");
      return;
    }
    savingRef.current = true;
    setSaving(true);
    setFailure(undefined);
    const req = request(level);
    try {
      const result = await saveAlta(req);
      const message = createdMessage(req.student, result);
      if (mode === "close") onCreated(message);
      else startNext(message);
    } catch (err) {
      if (err instanceof PlacementFailed) setCreatedId(err.studentId);
      setFailure(altaFailureMessage(err));
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  return { draft, set, chooseLevel, chooseGrade, errors, clearError, attempt, failure, done, saving, showMore, setShowMore, today, created: createdId !== undefined, submit };
}

export type AltaForm = ReturnType<typeof useAltaForm>;

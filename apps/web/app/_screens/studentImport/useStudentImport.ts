"use client";

import { useCallback, useRef, useState } from "react";

import {
  StudentImportError,
  commitStudentImport,
  previewStudentImport,
  type LevelChoice,
  type RowAction,
  type RowFieldError,
  type RowOverrides,
  type StudentImportRequest,
  type StudentImportResult,
} from "../../studentImport/api";
import { CUT_UNKNOWN, ROWS_INVALID, STALE_PREVIEW, cutWithCount, failureText } from "../../studentImport/copy";
import { decisionRows, withAction, withOverrides, type Decisions } from "../../studentImport/review";
import { pickFile, readAsBase64 } from "../../studentImport/file";
import { notAnsweredCount } from "../../studentImport/view";

export type ImportPhase = "upload" | "preview" | "committing" | "result";

interface Choices {
  levels: Record<string, LevelChoice>;
  capacities: Record<string, number>;
}

const NO_CHOICES: Choices = { levels: {}, capacities: {} };
const NO_DECISIONS: Decisions = {};
const NO_ROW_ERRORS: readonly RowFieldError[] = [];
const ROWS_INVALID_CODE = "IMPORT_ROWS_INVALID";

/**
 * File → preview → commit. Every choice (a level's destination, a new group's
 * capacity) re-asks the preview, and the commit re-sends the same file with the
 * same choices and the preview's planHash: the gateway keeps no state between.
 *
 * With the import review on (module_import_review) the office's decision per
 * row — a correction, an omission, a draft — is one more choice: each one
 * re-asks the preview and the commit carries the same ones.
 */
export function useStudentImport(baseURL: string, onNoCycle: () => void, review = false) {
  const [phase, setPhase] = useState<ImportPhase>("upload");
  const [reading, setReading] = useState<string | undefined>(undefined);
  const [notice, setNotice] = useState<string | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [preview, setPreview] = useState<StudentImportResult | undefined>(undefined);
  const [result, setResult] = useState<StudentImportResult | undefined>(undefined);
  const [cut, setCut] = useState<string | undefined>(undefined);
  const [focusUnresolved, setFocusUnresolved] = useState(0);
  const file = useRef<{ fileName: string; fileBase64: string } | undefined>(undefined);
  const choices = useRef<Choices>(NO_CHOICES);
  // A ref for the request, a state for the table: the same decisions.
  const decided = useRef<Decisions>(NO_DECISIONS);
  const [decisions, setDecisions] = useState<Decisions>(NO_DECISIONS);
  const [rowErrors, setRowErrors] = useState<readonly RowFieldError[]>(NO_ROW_ERRORS);

  const request = (): StudentImportRequest | undefined => {
    if (file.current === undefined) return undefined;
    const body: StudentImportRequest = { ...file.current, levels: choices.current.levels, capacities: choices.current.capacities };
    return review ? { ...body, rows: decisionRows(decided.current) } : body;
  };

  const refused = useCallback((err: unknown): void => {
    if (err instanceof StudentImportError && err.code === "NO_ACTIVE_CYCLE") {
      onNoCycle();
      return;
    }
    if (err instanceof StudentImportError && err.code === ROWS_INVALID_CODE) setRowErrors(err.rows);
    setError(failureText(err));
  }, [onNoCycle]);

  const setDecided = (next: Decisions): void => {
    decided.current = next;
    setDecisions(next);
  };

  // Each choice starts a preview; only the latest one's answer may paint the
  // screen, or an older answer arriving last would show a plan the office has
  // already changed (and its stale hash would fail the commit).
  const latest = useRef(0);

  const runPreview = useCallback(async (): Promise<void> => {
    const body = request();
    if (body === undefined) return;
    const mine = ++latest.current;
    try {
      const answer = await previewStudentImport(baseURL, body);
      if (mine !== latest.current) return;
      setPreview(answer);
      setPhase("preview");
      setRowErrors(NO_ROW_ERRORS);
      setError((shown) => (shown === ROWS_INVALID ? undefined : shown));
    } catch (err) {
      if (mine === latest.current) refused(err);
    }
  }, [baseURL, refused]);

  const chooseFiles = useCallback(async (files: readonly File[]): Promise<void> => {
    const picked = pickFile(files);
    setNotice(picked.notice);
    setError(picked.error);
    if (picked.file === undefined) return;
    setReading(picked.file.name);
    try {
      file.current = { fileName: picked.file.name, fileBase64: await readAsBase64(picked.file) };
    } catch {
      setError("No pudimos leer el archivo en este equipo. Intenta de nuevo.");
      return;
    } finally {
      setReading(undefined);
    }
    choices.current = NO_CHOICES;
    setDecided(NO_DECISIONS);
    setRowErrors(NO_ROW_ERRORS);
    await runPreview();
  }, [runPreview]);

  const decide = useCallback((row: number, action: RowAction | undefined): void => {
    setDecided(withAction(decided.current, row, action));
    void runPreview();
  }, [runPreview]);

  const correct = useCallback((row: number, overrides: RowOverrides): void => {
    setDecided(withOverrides(decided.current, row, overrides));
    void runPreview();
  }, [runPreview]);

  const choose = useCallback((fileName: string, choice: LevelChoice): void => {
    choices.current = { ...choices.current, levels: { ...choices.current.levels, [fileName]: choice } };
    setError(undefined);
    void runPreview();
  }, [runPreview]);

  const setCapacity = useCallback((key: string, capacity: number): void => {
    choices.current = { ...choices.current, capacities: { ...choices.current.capacities, [key]: capacity } };
    void runPreview();
  }, [runPreview]);

  const commit = useCallback(async (): Promise<void> => {
    const body = request();
    if (body === undefined || preview === undefined) return;
    setError(undefined);
    setPhase("committing");
    try {
      const done = await commitStudentImport(baseURL, { ...body, planHash: preview.planHash });
      const missing = notAnsweredCount(done.rows);
      setCut(missing > 0 ? cutWithCount(done.summary.new, done.summary.new + missing) : undefined);
      setResult(done);
      setPhase("result");
    } catch (err) {
      afterFailedCommit(err);
    }
  }, [baseURL, preview]);

  const afterFailedCommit = (err: unknown): void => {
    if (!(err instanceof StudentImportError)) {
      // The answer never came: nobody knows how many went in, but the same file is safe.
      file.current = undefined;
      setPreview(undefined);
      setPhase("upload");
      setError(CUT_UNKNOWN);
      return;
    }
    setPhase("preview");
    if (err.code === "PREVIEW_STALE") {
      setError(STALE_PREVIEW);
      void runPreview();
    } else if (err.code === "LEVELS_UNRESOLVED") {
      setFocusUnresolved((n) => n + 1);
      void runPreview();
    } else {
      refused(err);
    }
  };

  const startOver = useCallback((): void => {
    file.current = undefined;
    choices.current = NO_CHOICES;
    setDecided(NO_DECISIONS);
    setRowErrors(NO_ROW_ERRORS);
    setPreview(undefined);
    setResult(undefined);
    setCut(undefined);
    setError(undefined);
    setNotice(undefined);
    setPhase("upload");
  }, []);

  return { phase, reading, notice, error, preview, result, cut, focusUnresolved, decisions, rowErrors, chooseFiles, choose, setCapacity, decide, correct, commit, startOver };
}

export type StudentImportFlow = ReturnType<typeof useStudentImport>;

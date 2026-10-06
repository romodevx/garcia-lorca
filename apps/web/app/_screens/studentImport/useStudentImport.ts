"use client";

import { useCallback, useRef, useState } from "react";

import {
  StudentImportError,
  commitStudentImport,
  previewStudentImport,
  type LevelChoice,
  type StudentImportRequest,
  type StudentImportResult,
} from "../../studentImport/api";
import { CUT_UNKNOWN, STALE_PREVIEW, cutWithCount, failureText } from "../../studentImport/copy";
import { pickFile, readAsBase64 } from "../../studentImport/file";
import { notAnsweredCount } from "../../studentImport/view";

export type ImportPhase = "upload" | "preview" | "committing" | "result";

interface Choices {
  levels: Record<string, LevelChoice>;
  capacities: Record<string, number>;
}

const NO_CHOICES: Choices = { levels: {}, capacities: {} };

/**
 * File → preview → commit. Every choice (a level's destination, a new group's
 * capacity) re-asks the preview, and the commit re-sends the same file with the
 * same choices and the preview's planHash: the gateway keeps no state between.
 */
export function useStudentImport(baseURL: string, onNoCycle: () => void) {
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

  const request = (): StudentImportRequest | undefined =>
    file.current === undefined ? undefined : { ...file.current, levels: choices.current.levels, capacities: choices.current.capacities };

  const refused = useCallback((err: unknown): void => {
    if (err instanceof StudentImportError && err.code === "NO_ACTIVE_CYCLE") {
      onNoCycle();
      return;
    }
    setError(failureText(err));
  }, [onNoCycle]);

  const runPreview = useCallback(async (): Promise<void> => {
    const body = request();
    if (body === undefined) return;
    try {
      setPreview(await previewStudentImport(baseURL, body));
      setPhase("preview");
    } catch (err) {
      refused(err);
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
    await runPreview();
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
    setPreview(undefined);
    setResult(undefined);
    setCut(undefined);
    setError(undefined);
    setNotice(undefined);
    setPhase("upload");
  }, []);

  return { phase, reading, notice, error, preview, result, cut, focusUnresolved, chooseFiles, choose, setCapacity, commit, startOver };
}

export type StudentImportFlow = ReturnType<typeof useStudentImport>;

"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { API_URL } from "../../api-url";
import type { IssueField } from "../../studentImport/api";
import { acceptImportIssue, listImportIssues, type StudentImportIssue } from "../../students/importIssues";

const ACCEPT_FAILED = "No se pudo cerrar el aviso. Intenta de nuevo.";

export interface ImportIssuesState {
  /** The open issues of one field, in the order students lists them. */
  of: (field: IssueField) => StudentImportIssue[];
  /** «Está bien así» for every open issue of the field: gone at once, back if refused. */
  accept: (field: IssueField) => Promise<void>;
  /** After a save, which closes the corrected issues server-side. */
  reload: () => void;
  /** Why the last «Está bien así» did not go through. */
  acceptError: string | undefined;
  /** The group a draft's import meant it for (its curp issue). */
  intendedGroupId: string | undefined;
}

/**
 * The pupil's open import issues (module_import_review). Off, nothing is read:
 * the routes 404 while the flag is off, and the file behaves as before.
 */
export function useImportIssues(studentId: string, enabled: boolean): ImportIssuesState {
  const [issues, setIssues] = useState<StudentImportIssue[]>([]);
  const [acceptError, setAcceptError] = useState<string | undefined>(undefined);
  const latest = useRef(0);

  const reload = useCallback((): void => {
    if (!enabled) return;
    const mine = ++latest.current;
    listImportIssues(API_URL, studentId)
      .then((open) => mine === latest.current && setIssues(open))
      // Unreadable issues warn nothing; the file itself still works.
      .catch(() => undefined);
  }, [studentId, enabled]);

  useEffect(() => {
    reload();
  }, [reload]);

  const of = useCallback((field: IssueField) => issues.filter((i) => i.field === field), [issues]);

  const accept = useCallback(async (field: IssueField): Promise<void> => {
    const closing = issues.filter((i) => i.field === field);
    if (closing.length === 0) return;
    setAcceptError(undefined);
    setIssues((all) => all.filter((i) => i.field !== field));
    const results = await Promise.allSettled(closing.map((i) => acceptImportIssue(API_URL, studentId, i.id)));
    const failed = closing.filter((_, at) => results[at]?.status === "rejected");
    if (failed.length === 0) return;
    // A reload may have brought some back already: merge by id, never twice.
    setIssues((all) => [...all, ...failed.filter((f) => !all.some((i) => i.id === f.id))]);
    setAcceptError(ACCEPT_FAILED);
  }, [issues, studentId]);

  const intendedGroupId = issues.find((i) => i.field === "curp" && i.intendedGroupId !== undefined && i.intendedGroupId !== "")?.intendedGroupId;

  return { of, accept, reload, acceptError, intendedGroupId };
}

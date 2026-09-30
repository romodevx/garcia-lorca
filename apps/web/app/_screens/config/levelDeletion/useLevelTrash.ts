"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { API_URL } from "../../../api-url";
import { ConfigError, listLevelTrash, restoreLevel, type LevelRestoreResult, type LevelTrashItem } from "../../../config/api";

export type TrashLoad = { kind: "loading" } | { kind: "failed" } | { kind: "ready"; items: LevelTrashItem[] };

export interface RestoreOutcome {
  levelName: string;
  result: LevelRestoreResult;
}

export interface LevelTrash {
  load: TrashLoad;
  confirming: LevelTrashItem | undefined;
  restoreError: string | undefined;
  restoring: boolean;
  outcome: RestoreOutcome | undefined;
  ask: (item: LevelTrashItem) => void;
  cancel: () => void;
  restore: () => Promise<void>;
}

function refusalMessage(err: unknown, levelName: string): string {
  if (err instanceof ConfigError && err.code === "LEVEL_NAME_TAKEN") {
    return `Ya existe otro nivel llamado ${levelName}. Cambia el nombre de ese nivel y vuelve a intentarlo.`;
  }
  if (err instanceof ConfigError && err.code === "TRASH_EXPIRED") {
    return `Pasaron los 7 días: ${levelName} ya no se puede restaurar.`;
  }
  // A half-done restore, a missing part, a level being deleted: the server's
  // own sentence says what happened and what a retry does.
  if (err instanceof ConfigError && (err.code !== undefined || err.status >= 500) && err.message !== `HTTP ${err.status}`) {
    return err.message;
  }
  return "No se pudo restaurar el nivel. Revisa tu conexión e intenta de nuevo.";
}

/** The trash of deleted levels: the list, the one confirmation, and the restore. */
export function useLevelTrash(onRestored: () => void): LevelTrash {
  const [load, setLoad] = useState<TrashLoad>({ kind: "loading" });
  const [confirming, setConfirming] = useState<LevelTrashItem | undefined>(undefined);
  const [restoreError, setRestoreError] = useState<string | undefined>(undefined);
  const [restoring, setRestoring] = useState(false);
  const [outcome, setOutcome] = useState<RestoreOutcome | undefined>(undefined);
  const inFlight = useRef(false);

  const reload = useCallback((): void => {
    listLevelTrash(API_URL)
      .then((items) => setLoad({ kind: "ready", items }))
      .catch(() => setLoad({ kind: "failed" }));
  }, []);

  useEffect(reload, [reload]);

  const restore = async (): Promise<void> => {
    if (inFlight.current || confirming === undefined) return;
    inFlight.current = true;
    setRestoring(true);
    setRestoreError(undefined);
    try {
      const result = await restoreLevel(API_URL, confirming.batchId);
      setOutcome({ levelName: confirming.levelName, result });
      setConfirming(undefined);
      onRestored();
      reload();
    } catch (err) {
      setRestoreError(refusalMessage(err, confirming.levelName));
      if (err instanceof ConfigError && err.code === "TRASH_EXPIRED") reload();
    } finally {
      inFlight.current = false;
      setRestoring(false);
    }
  };

  return {
    load, confirming, restoreError, restoring, outcome, restore,
    ask: (item) => {
      setRestoreError(undefined);
      setConfirming(item);
    },
    cancel: () => setConfirming(undefined),
  };
}

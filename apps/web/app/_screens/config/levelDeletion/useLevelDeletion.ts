"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { API_URL } from "../../../api-url";
import { ConfigError, deleteLevel, getLevelDeletionPreview, type LevelDeletionPreview } from "../../../config/api";
import { longDay, namesMatch } from "./copy";

export type Step = 1 | 2 | 3;

export type PreviewLoad =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "ready"; preview: LevelDeletionPreview };

export const STALE_NOTICE = "El nivel cambió mientras confirmabas. Revisa el resumen actualizado y vuelve a confirmar.";
const EMPTY_PASSWORD = "Escribe tu contraseña.";
const WRONG_PASSWORD = "Contraseña incorrecta.";
const FORBIDDEN = "Sólo el director puede eliminar niveles.";
const GONE = "Este nivel ya no existe. Puede que alguien más lo haya eliminado.";
const NETWORK = "No se pudo eliminar el nivel. Revisa tu conexión e intenta de nuevo.";

const nameMismatch = (levelName: string): string => `El nombre no coincide con ${levelName}.`;

export interface LevelDeletion {
  load: PreviewLoad;
  step: Step;
  notice: string | undefined;
  name: string;
  nameError: string | undefined;
  password: string;
  passwordError: string | undefined;
  submitError: string | undefined;
  submitting: boolean;
  setName: (v: string) => void;
  setPassword: (v: string) => void;
  retry: () => void;
  back: () => void;
  confirmSummary: () => void;
  checkName: () => boolean;
  confirmName: () => void;
  submit: () => Promise<void>;
}

/**
 * The three confirmations of deleting a level, and every way the server can
 * refuse the last one. The step lives here, not in the page, because a refusal
 * decides where the director lands: a stale summary starts over, a name the
 * server rejects goes back to the name, a wrong password stays put.
 */
export function useLevelDeletion(levelId: string, onDeleted: (message: string) => void): LevelDeletion {
  const [load, setLoad] = useState<PreviewLoad>({ kind: "loading" });
  const [step, setStep] = useState<Step>(1);
  const [notice, setNotice] = useState<string | undefined>(undefined);
  const [name, setNameValue] = useState("");
  const [nameError, setNameError] = useState<string | undefined>(undefined);
  const [password, setPasswordValue] = useState("");
  const [passwordError, setPasswordError] = useState<string | undefined>(undefined);
  const [submitError, setSubmitError] = useState<string | undefined>(undefined);
  const [submitting, setSubmitting] = useState(false);
  // State lags a render behind a double click; a ref does not.
  const inFlight = useRef(false);

  const fetchPreview = useCallback((): void => {
    setLoad({ kind: "loading" });
    getLevelDeletionPreview(API_URL, levelId)
      .then((preview) => setLoad({ kind: "ready", preview }))
      .catch(() => setLoad({ kind: "failed" }));
  }, [levelId]);

  useEffect(fetchPreview, [fetchPreview]);

  const levelName = load.kind === "ready" ? load.preview.levelName : "";

  const setName = (v: string): void => {
    setNameValue(v);
    if (nameError !== undefined && namesMatch(v, levelName)) setNameError(undefined);
  };
  const setPassword = (v: string): void => {
    setPasswordValue(v);
    setPasswordError(undefined);
  };

  const checkName = (): boolean => {
    const ok = namesMatch(name, levelName);
    setNameError(ok ? undefined : nameMismatch(levelName));
    return ok;
  };

  const startOver = (): void => {
    setNameValue("");
    setPasswordValue("");
    setNotice(STALE_NOTICE);
    setStep(1);
    fetchPreview();
  };

  const refuse = (err: unknown): void => {
    setPasswordValue("");
    if (!(err instanceof ConfigError)) return setSubmitError(NETWORK);
    if (err.code === "PREVIEW_STALE") return startOver();
    if (err.code === "NAME_MISMATCH") {
      setNameError(nameMismatch(levelName));
      return setStep(2);
    }
    if (err.code === "REAUTH_FAILED") return setPasswordError(WRONG_PASSWORD);
    if (err.status === 403) return setSubmitError(FORBIDDEN);
    if (err.status === 404) return setSubmitError(GONE);
    setSubmitError(NETWORK);
  };

  const submit = async (): Promise<void> => {
    if (inFlight.current || load.kind !== "ready") return;
    if (password === "") {
      setPasswordError(EMPTY_PASSWORD);
      return;
    }
    inFlight.current = true;
    setSubmitting(true);
    setSubmitError(undefined);
    try {
      // What the director TYPED: the server verifies it too (trimmed, any case).
      const done = await deleteLevel(API_URL, levelId, { confirmName: name.trim(), confirmToken: load.preview.confirmToken, password });
      onDeleted(`Nivel eliminado. Puedes restaurarlo desde la Papelera hasta el ${longDay(done.restorableUntil)}.`);
    } catch (err) {
      refuse(err);
    } finally {
      inFlight.current = false;
      setSubmitting(false);
    }
  };

  return {
    load, step, notice, name, nameError, password, passwordError, submitError, submitting,
    setName, setPassword, retry: fetchPreview, checkName, submit,
    back: () => {
      setSubmitError(undefined);
      setStep((s) => (s === 3 ? 2 : 1));
    },
    confirmSummary: () => {
      setNotice(undefined);
      setStep(2);
    },
    confirmName: () => {
      if (checkName()) setStep(3);
    },
  };
}

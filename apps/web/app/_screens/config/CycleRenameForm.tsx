"use client";

import { useEffect, useId, useRef, useState, type CSSProperties, type FormEvent, type KeyboardEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { ConfigError } from "../../config/api";
import { ErrorAlert } from "./parts";

const LOCALE = "es-MX";

export interface CycleRenameFormProps {
  /** The name as saved. */
  name: string;
  /** Saves the new (trimmed, non-empty) name. Rejects to keep the form open. */
  onRename: (name: string) => Promise<void>;
  onDone: () => void;
}

/** The server's sentence for a 400/409; a plain one otherwise. Never a code. */
function renameFailure(cause: unknown, name: string): string {
  if (cause instanceof ConfigError && cause.detail !== "" && (cause.isConflict || cause.isInvalid)) return cause.detail;
  return `No se pudo renombrar el ciclo ${name}. Intenta de nuevo.`;
}

/**
 * Renames a school year where it stands (QA CC-103): one field, so inline, not
 * a drawer. Enter or Guardar saves, Esc cancels, a refusal stays on the field.
 * Upper case, as the name is captured when the year is created.
 */
export function CycleRenameForm({ name, onRename, onDone }: CycleRenameFormProps): ReactElement {
  const [draft, setDraft] = useState(name);
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const saving = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const errorId = useId();

  useEffect(() => inputRef.current?.focus(), []);

  const save = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (saving.current) return;
    const next = draft.trim();
    if (next === "") {
      setError("Escribe el nombre del ciclo.");
      inputRef.current?.focus();
      return;
    }
    if (next === name) {
      onDone();
      return;
    }
    saving.current = true;
    setBusy(true);
    try {
      await onRename(next);
      onDone();
    } catch (cause) {
      setError(renameFailure(cause, name));
      inputRef.current?.focus();
    } finally {
      saving.current = false;
      setBusy(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>): void => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    e.stopPropagation();
    onDone();
  };

  return (
    <div style={{ marginBottom: 16 }}>
      <form onSubmit={(e) => void save(e)} aria-label={`Renombrar ciclo ${name}`} style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
        <input
          ref={inputRef}
          aria-label={`Nuevo nombre del ciclo ${name}`}
          aria-invalid={error !== undefined}
          aria-describedby={error !== undefined ? errorId : undefined}
          value={draft}
          maxLength={80}
          autoComplete="off"
          onChange={(e) => {
            setDraft(e.target.value.toLocaleUpperCase(LOCALE));
            if (error !== undefined) setError(undefined);
          }}
          onKeyDown={onKeyDown}
          style={inputStyle}
        />
        <Btn type="submit" icon="check" busy={busy} style={{ height: 44 }}>{busy ? "Guardando…" : "Guardar"}</Btn>
        <Btn kind="ghost" onClick={onDone} style={{ height: 44 }}>Cancelar</Btn>
      </form>
      {error !== undefined && <div id={errorId}><ErrorAlert message={error} /></div>}
    </div>
  );
}

const inputStyle: CSSProperties = {
  height: 44,
  padding: "0 14px",
  borderRadius: "var(--r-pill)",
  border: "1.5px solid var(--forest)",
  background: "var(--surface)",
  fontSize: 16,
  color: "var(--text)",
  minWidth: 160,
};

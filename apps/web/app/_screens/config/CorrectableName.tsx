"use client";

import { useEffect, useRef, useState, type CSSProperties, type FormEvent, type KeyboardEvent, type ReactElement, type ReactNode } from "react";

import { ConfirmDialog } from "../../_design/ConfirmDialog";
import { Icon } from "../../_design/Icon";
import { ConfigError } from "../../config/api";
import { ErrorAlert } from "./parts";

const LOCALE = "es-MX";

/** Renaming and deleting a catalog row by id; given only to config:write. */
export interface NameCorrections {
  rename: (id: string, name: string) => Promise<void>;
  remove: (id: string) => Promise<void>;
}

export interface CorrectableNameProps {
  /** The name as saved. */
  name: string;
  /** What it is, in lower case: "grado", "grupo". Every label and sentence uses it. */
  noun: string;
  /** Saves the new (trimmed, non-empty, changed) name. Rejects to keep the editor open. */
  onRename: (name: string) => Promise<void>;
  /** Deletes it. The caller drops it from its list on success. */
  onDelete: () => Promise<void>;
  /**
   * After a delete went through: `next` is where the focus belongs — the next
   * row's rename control, else the previous one's — or null when none is left.
   */
  onDeleted?: (next: HTMLElement | null) => void;
  /** The dialog's sentence: what exactly goes ("Se borra el grado 3° de PRIMARIA."). */
  deleteDescription: string;
  /** How the name is shown when not being edited (a chip, a pill). */
  children: ReactNode;
  /**
   * Where it is, for the pencil's and the trash's names when the same name
   * repeats on screen («de PRIMARIA»: every level has a 1°). Absent, the names
   * stay «Renombrar grado 1°».
   */
  qualifier?: string;
}

/** The rename went through in config but the pupils' files did not follow: saving again finishes it. */
const RELABEL_PENDING = "GROUP_RELABEL_PENDING";

/** Marks each row's rename control, so a delete can hand the focus to a neighbour's. */
const RENAME_CONTROL = "[data-correct-rename], input";

/**
 * The server's own sentence when it refused with one (a 409 names what blocks
 * the delete and how many; a 400 says what is wrong with the name; a 503
 * GROUP_RELABEL_PENDING says to save again); a plain sentence otherwise. Never a code.
 */
function refusalMessage(cause: unknown, fallback: string): string {
  if (!(cause instanceof ConfigError) || cause.detail === "") return fallback;
  if (cause.isConflict || cause.isInvalid || cause.code === RELABEL_PENDING) return cause.detail;
  return fallback;
}

/** The rename control of the row after this one, else of the one before; null when it is alone. */
function neighbourControl(item: HTMLElement | null): HTMLElement | null {
  const inRow = (row: Element | null | undefined) => row?.querySelector<HTMLElement>(RENAME_CONTROL) ?? null;
  return inRow(item?.nextElementSibling) ?? inRow(item?.previousElementSibling);
}

/**
 * One catalog name that can be corrected where it stands: the pencil turns it
 * into an input (Enter or Guardar saves, Esc cancels), the trash asks first —
 * deleting a grade or a group does not undo — and any refusal is shown right
 * here, in the server's words.
 */
export function CorrectableName({ name, noun, onRename, onDelete, onDeleted, deleteDescription, children, qualifier }: CorrectableNameProps): ReactElement {
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const deleting = useRef(false);
  const itemRef = useRef<HTMLLIElement>(null);
  const label = `${noun} ${name}`;
  const controlLabel = qualifier === undefined ? label : `${label} ${qualifier}`;

  const remove = async (): Promise<void> => {
    setConfirming(false);
    if (deleting.current) return;
    deleting.current = true;
    setError(undefined);
    // Read before the delete: once it lands this row, and its place among its siblings, is gone.
    const next = neighbourControl(itemRef.current);
    try {
      await onDelete();
      onDeleted?.(next);
    } catch (cause) {
      setError(refusalMessage(cause, `No se pudo borrar el ${label}. Intenta de nuevo.`));
    } finally {
      deleting.current = false;
    }
  };

  if (editing) {
    return <NameEditor name={name} noun={noun} onRename={onRename} onDone={() => setEditing(false)} />;
  }

  return (
    <li ref={itemRef} style={itemStyle}>
      <span style={{ display: "inline-flex", alignItems: "center", gap: 2 }}>
        {children}
        <button type="button" data-correct-rename="" aria-label={`Renombrar ${controlLabel}`} onClick={() => { setError(undefined); setEditing(true); }} style={iconButton}>
          <Icon name="edit" size={16} />
        </button>
        <button type="button" aria-label={`Borrar ${controlLabel}`} onClick={() => { setError(undefined); setConfirming(true); }} style={iconButton}>
          <Icon name="trash" size={16} />
        </button>
      </span>
      {error !== undefined && <ErrorAlert message={error} />}
      {confirming && (
        <ConfirmDialog title={`¿Borrar el ${label}?`} confirmLabel={`Borrar ${noun}`} icon="trash" onConfirm={() => void remove()} onCancel={() => setConfirming(false)}>
          <p style={{ margin: 0 }}>{deleteDescription}</p>
          <p style={{ margin: "8px 0 0", color: "var(--text-muted)" }}>No se puede deshacer.</p>
        </ConfirmDialog>
      )}
    </li>
  );
}

interface NameEditorProps {
  name: string;
  noun: string;
  onRename: (name: string) => Promise<void>;
  onDone: () => void;
}

/** The input that replaces the name while it is being corrected. */
function NameEditor({ name, noun, onRename, onDone }: NameEditorProps): ReactElement {
  const [draft, setDraft] = useState(name);
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const saving = useRef(false);
  // After a refusal the list is re-read and may already read the typed name
  // (GROUP_RELABEL_PENDING): saving it again must still be sent, to finish.
  const refused = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const errorId = `rename-error-${noun}-${name}`;

  useEffect(() => inputRef.current?.focus(), []);

  const save = async (e?: FormEvent): Promise<void> => {
    e?.preventDefault();
    if (saving.current) return;
    const next = draft.trim();
    if (next === "") {
      setError(`Escribe el nombre del ${noun}.`);
      inputRef.current?.focus();
      return;
    }
    if (next === name && !refused.current) {
      onDone();
      return;
    }
    saving.current = true;
    setBusy(true);
    try {
      await onRename(next);
      onDone();
    } catch (cause) {
      refused.current = true;
      setError(refusalMessage(cause, `No se pudo renombrar el ${noun} ${name}. Intenta de nuevo.`));
      inputRef.current?.focus();
    } finally {
      saving.current = false;
      setBusy(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>): void => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    // The dialog-less Esc must not also close a drawer around the panel.
    e.stopPropagation();
    onDone();
  };

  return (
    <li style={itemStyle}>
      <form onSubmit={(e) => void save(e)} aria-label={`Renombrar ${noun} ${name}`} style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
        <input
          ref={inputRef}
          aria-label={`Nuevo nombre de ${noun} ${name}`}
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
        <button type="submit" aria-busy={busy} style={saveStyle}>
          <Icon name="check" size={16} />
          {busy ? "Guardando…" : "Guardar"}
        </button>
        <button type="button" onClick={onDone} style={cancelStyle}>Cancelar</button>
      </form>
      {error !== undefined && <div id={errorId}><ErrorAlert message={error} /></div>}
    </li>
  );
}

const itemStyle: CSSProperties = { display: "grid", justifyItems: "start", gap: 0, listStyle: "none" };

const iconButton: CSSProperties = {
  display: "grid",
  placeItems: "center",
  width: 44,
  height: 44,
  borderRadius: "var(--r-pill)",
  color: "var(--text-muted)",
  background: "transparent",
};

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

const saveStyle: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  minHeight: 44,
  padding: "0 16px",
  borderRadius: "var(--r-pill)",
  background: "var(--primary)",
  color: "var(--primary-ink)",
  fontWeight: 600,
  fontSize: 14.5,
};

const cancelStyle: CSSProperties = {
  minHeight: 44,
  padding: "0 8px",
  background: "transparent",
  color: "var(--text-muted)",
  fontWeight: 600,
  fontSize: 14.5,
};

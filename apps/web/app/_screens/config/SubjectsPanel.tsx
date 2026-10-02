"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { UndoToast } from "../../_design/UndoToast";
import { TextField, useFocusFirstInvalid } from "../../_design/form";
import {
  ConfigError,
  createSubject,
  createSubjectInGrades,
  deleteSubject,
  listSubjects,
  renameSubject,
  type GradeLevel,
  type Level,
  type Subject,
  type SubjectsInGrades,
} from "../../config/api";
import { EmptyHint, ErrorAlert, Section, SelectChip } from "./parts";

export interface SubjectsPanelProps {
  level: Level;
  /** The grades of `level`, in any order. */
  gradeLevels: readonly GradeLevel[];
  /** The grade chosen on the page, if any: the new subject goes there by default. */
  selectedGradeLevelId?: string;
}

const LOCALE = "es-MX";
const NO_NAME = "Escribe el nombre de la materia.";
const NO_GRADE = "Elige al menos un grado.";
/** How long the undo offer stays after a delete. */
const UNDO_MS = 8_000;

/**
 * Las materias de un nivel, grado por grado (T13).
 *
 * Una materia es de UN grado: "Matemáticas" de 1° y de 2° son dos renglones, y
 * a un grupo sólo se le asignan las de su grado. Aquí se ven todas las del
 * nivel juntas, y el alta pide el nombre una vez y los grados donde va —
 * crea una por grado. Dos campos: renglón en el lugar, no cajón.
 */
export function SubjectsPanel({ level, gradeLevels, selectedGradeLevelId }: SubjectsPanelProps): ReactElement {
  const grades = useMemo(() => [...gradeLevels].sort((a, b) => a.ordinal - b.ordinal), [gradeLevels]);
  const gradeName = useCallback((id: string): string => grades.find((g) => g.id === id)?.name ?? "", [grades]);
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [error, setError] = useState<string | undefined>(undefined);
  const [notice, setNotice] = useState<string | undefined>(undefined);
  const [deleted, setDeleted] = useState<Subject | undefined>(undefined);
  // Deletes in flight: a double click must not send a second DELETE (its 404
  // would read as a failure of a delete that worked).
  const deleting = useRef(new Set<string>());

  // ONE request for the school's subjects, kept to this level's grades. A
  // response that arrives after a newer one (a grade was just added) is dropped.
  const gradeKey = grades.map((g) => g.id).join(",");
  useEffect(() => {
    const ids = new Set(gradeKey === "" ? [] : gradeKey.split(","));
    let active = true;
    listSubjects(API_URL, "")
      .then((all) => active && setSubjects(all.filter((s) => ids.has(s.gradeLevelId))))
      .catch(() => active && setError("No se pudieron cargar las materias de este nivel."));
    return () => {
      active = false;
    };
  }, [gradeKey]);

  // The undo offer leaves on its own (≥ 8 s, like every undo toast).
  useEffect(() => {
    if (deleted === undefined) return;
    const timer = setTimeout(() => setDeleted(undefined), UNDO_MS);
    return () => clearTimeout(timer);
  }, [deleted]);

  const onCreated = (name: string, res: SubjectsInGrades): void => {
    setSubjects((prev) => [...prev, ...res.created]);
    setNotice(createdNotice(name, res, gradeName));
    setError(res.failed.length > 0 ? `No se pudo agregar a ${joinNames(res.failed.map(gradeName))}. Intenta de nuevo.` : undefined);
  };

  const onRenamed = (renamed: Subject): void => {
    setSubjects((prev) => prev.map((s) => (s.id === renamed.id ? renamed : s)));
  };

  /*
   * The delete goes out AT ONCE, not after the undo window: the server is the
   * one that knows whether the subject was ever in a teacher's load, and that
   * refusal has to reach the person while they are still looking — not eight
   * seconds later, or never if they already moved to another level. Undo puts
   * it back as a new row, which is harmless precisely because the server only
   * deletes a subject nothing ever pointed at.
   */
  const remove = async (s: Subject): Promise<void> => {
    if (deleting.current.has(s.id)) return;
    deleting.current.add(s.id);
    setError(undefined);
    try {
      await deleteSubject(API_URL, s.id);
      setSubjects((prev) => prev.filter((x) => x.id !== s.id));
      setDeleted(s);
    } catch (cause) {
      setError(
        cause instanceof ConfigError && cause.isConflict
          ? `${s.name} ya estuvo en una carga académica: se puede renombrar, pero no eliminar.`
          : `No se pudo eliminar ${s.name}.`,
      );
    } finally {
      deleting.current.delete(s.id);
    }
  };

  const undo = async (s: Subject): Promise<void> => {
    setDeleted(undefined);
    try {
      const back = await createSubject(API_URL, { gradeLevelId: s.gradeLevelId, name: s.name });
      setSubjects((prev) => [...prev, back]);
    } catch {
      setError(`No se pudo devolver ${s.name}. Agrégala de nuevo.`);
    }
  };

  return (
    <Section title="Materias" subtitle={`De ${level.name}, por grado. Cada grado tiene las suyas.`}>
      <NewSubjectForm grades={grades} selectedGradeLevelId={selectedGradeLevelId} onCreated={onCreated} />
      {notice !== undefined && (
        <p role="status" style={{ margin: "10px 0 0", fontSize: 13.5, color: "var(--text-muted)", fontWeight: 600 }}>
          {notice}
        </p>
      )}
      {error !== undefined && <ErrorAlert message={error} />}

      {grades.length === 0 ? (
        <EmptyHint>Este nivel aún no tiene grados.</EmptyHint>
      ) : (
        <div style={{ display: "grid", gap: 14, marginTop: 18 }}>
          {grades.map((g) => (
            <GradeSubjects
              key={g.id}
              grade={g}
              subjects={subjects.filter((s) => s.gradeLevelId === g.id).sort((a, b) => a.name.localeCompare(b.name, LOCALE))}
              onRenamed={onRenamed}
              onRemove={(s) => void remove(s)}
            />
          ))}
        </div>
      )}

      {deleted !== undefined && <UndoToast message={`Se eliminó ${deleted.name}`} onUndo={() => void undo(deleted)} />}
    </Section>
  );
}

/** "ARTES se agregó a 1° y 3°. 2° ya la tenía." */
function createdNotice(name: string, res: SubjectsInGrades, gradeName: (id: string) => string): string {
  const parts: string[] = [];
  if (res.created.length > 0) parts.push(`${name} se agregó a ${joinNames(res.created.map((s) => gradeName(s.gradeLevelId)))}.`);
  if (res.existing.length > 0) {
    const plural = res.existing.length > 1;
    parts.push(`${joinNames(res.existing.map(gradeName))} ya la ${plural ? "tenían" : "tenía"}.`);
  }
  return parts.join(" ");
}

function joinNames(names: readonly string[]): string {
  if (names.length <= 1) return names.join("");
  return `${names.slice(0, -1).join(", ")} y ${names[names.length - 1]}`;
}

interface NewSubjectFormProps {
  grades: readonly GradeLevel[];
  selectedGradeLevelId?: string;
  onCreated: (name: string, res: SubjectsInGrades) => void;
}

/**
 * Nombre y grados. Arranca con el grado elegido en la página o, sin él, con
 * todos los del nivel; después de guardar conserva los grados y limpia el
 * nombre, para dar de alta la siguiente sin volver a elegir.
 */
function NewSubjectForm({ grades, selectedGradeLevelId, onCreated }: NewSubjectFormProps): ReactElement {
  const [name, setName] = useState("");
  // Until the person touches a grade the choice is DERIVED: the grades may
  // arrive after the form (the catalog loads them late), and a default frozen at
  // mount would start with nothing chosen.
  const [picked, setPicked] = useState<ReadonlySet<string> | undefined>(undefined);
  const chosen: ReadonlySet<string> =
    picked ?? new Set(selectedGradeLevelId !== undefined ? [selectedGradeLevelId] : grades.map((g) => g.id));
  const [nameError, setNameError] = useState<string | undefined>(undefined);
  const [gradeError, setGradeError] = useState<string | undefined>(undefined);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const saving = useRef(false);
  const formRef = useRef<HTMLFormElement>(null);
  const focusFirstInvalid = useFocusFirstInvalid();

  const all = grades.length > 0 && grades.every((g) => chosen.has(g.id));
  const toggle = (id: string): void => {
    const next = new Set(chosen);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setPicked(next);
    setGradeError(undefined);
  };

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (saving.current) return;
    const trimmed = name.trim();
    const gradeIds = grades.filter((g) => chosen.has(g.id)).map((g) => g.id);
    if (trimmed === "") setNameError(NO_NAME);
    if (gradeIds.length === 0) setGradeError(NO_GRADE);
    if (trimmed === "") {
      focusFirstInvalid();
      return;
    }
    if (gradeIds.length === 0) return;
    saving.current = true;
    setFailure(undefined);
    try {
      const res = await createSubjectInGrades(API_URL, trimmed, gradeIds);
      onCreated(trimmed, res);
      setName("");
      formRef.current?.querySelector<HTMLInputElement>("input")?.focus();
    } catch {
      setFailure("No se pudo agregar la materia. Intenta de nuevo.");
    } finally {
      saving.current = false;
    }
  };

  return (
    <form ref={formRef} aria-label="Nueva materia" onSubmit={submit} style={{ display: "grid", gap: 12 }}>
      <TextField
        label="Nombre de la materia"
        value={name}
        onChange={setName}
        uppercase
        required
        error={nameError}
        onClearError={() => setNameError(undefined)}
        help="Como aparecerá en la boleta. Se crea una en cada grado que elijas."
      />
      <fieldset style={{ border: "none", padding: 0, margin: 0, minWidth: 0 }}>
        <legend style={{ fontSize: 13, fontWeight: 600, color: "var(--text-muted)", marginBottom: 6 }}>Grados</legend>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          <SelectChip selected={all} onClick={() => { setPicked(new Set(all ? [] : grades.map((g) => g.id))); setGradeError(undefined); }}>
            Todos los grados
          </SelectChip>
          {grades.map((g) => (
            <SelectChip key={g.id} selected={chosen.has(g.id)} onClick={() => toggle(g.id)}>
              {g.name}
            </SelectChip>
          ))}
        </div>
        {gradeError !== undefined && <ErrorAlert message={gradeError} />}
      </fieldset>
      <div>
        <Btn type="submit" icon="plus">Agregar materia</Btn>
      </div>
      {failure !== undefined && <ErrorAlert message={failure} />}
    </form>
  );
}

interface GradeSubjectsProps {
  grade: GradeLevel;
  subjects: readonly Subject[];
  onRenamed: (s: Subject) => void;
  onRemove: (s: Subject) => void;
}

function GradeSubjects({ grade, subjects, onRenamed, onRemove }: GradeSubjectsProps): ReactElement {
  return (
    <section aria-label={`Materias de ${grade.name}`} style={{ borderTop: "1px solid var(--border)", paddingTop: 12 }}>
      <h3 style={{ fontSize: 14, fontWeight: 700, color: "var(--text)", marginBottom: 8 }}>{grade.name}</h3>
      {subjects.length === 0 ? (
        <EmptyHint>Este grado aún no tiene materias.</EmptyHint>
      ) : (
        <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "flex", flexWrap: "wrap", gap: 8 }}>
          {subjects.map((s) => (
            <SubjectChip key={s.id} subject={s} gradeName={grade.name} onRenamed={onRenamed} onRemove={onRemove} />
          ))}
        </ul>
      )}
    </section>
  );
}

interface SubjectChipProps {
  subject: Subject;
  gradeName: string;
  onRenamed: (s: Subject) => void;
  onRemove: (s: Subject) => void;
}

/** One subject: its name, and rename / delete beside it. Renaming happens in the chip. */
function SubjectChip({ subject, gradeName, onRenamed, onRemove }: SubjectChipProps): ReactElement {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(subject.name);
  const [error, setError] = useState<string | undefined>(undefined);
  const saving = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing) inputRef.current?.focus();
  }, [editing]);

  const cancel = (): void => {
    setEditing(false);
    setDraft(subject.name);
    setError(undefined);
  };

  const save = async (): Promise<void> => {
    if (saving.current) return;
    const name = draft.trim();
    if (name === "" || name === subject.name) {
      cancel();
      return;
    }
    saving.current = true;
    try {
      onRenamed(await renameSubject(API_URL, subject.id, name));
      setEditing(false);
      setError(undefined);
    } catch (cause) {
      setError(
        cause instanceof ConfigError && cause.isConflict
          ? `${gradeName} ya tiene una materia con ese nombre.`
          : "No se pudo renombrar la materia. Intenta de nuevo.",
      );
    } finally {
      saving.current = false;
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>): void => {
    if (e.key === "Enter") {
      e.preventDefault();
      void save();
    }
    if (e.key === "Escape") {
      e.preventDefault();
      cancel();
    }
  };

  if (editing) {
    return (
      <li style={{ display: "grid", gap: 4 }}>
        <input
          ref={inputRef}
          aria-label={`Nuevo nombre de ${subject.name}`}
          aria-invalid={error !== undefined}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value.toLocaleUpperCase(LOCALE));
            setError(undefined);
          }}
          onKeyDown={onKeyDown}
          // Leaving with nothing changed closes it; with a new name typed it stays
          // open — walking away must not throw the edit away, nor send it.
          onBlur={() => {
            if (draft.trim() === subject.name) cancel();
          }}
          style={{ height: 44, padding: "0 14px", borderRadius: "var(--r-pill)", border: "1.5px solid var(--forest)", background: "var(--surface)", fontSize: 16, color: "var(--text)", minWidth: 200 }}
        />
        {error !== undefined && <ErrorAlert message={error} />}
      </li>
    );
  }

  return (
    <li style={{ display: "inline-flex", alignItems: "center", gap: 2, padding: "0 4px 0 14px", minHeight: 44, borderRadius: "var(--r-pill)", background: "var(--surface-2)", fontSize: 14, fontWeight: 600, color: "var(--text)" }}>
      <span>{subject.name}</span>
      <button type="button" aria-label={`Renombrar ${subject.name}`} onClick={() => setEditing(true)} style={iconButton}>
        <Icon name="edit" size={16} />
      </button>
      <button type="button" aria-label={`Eliminar ${subject.name}`} onClick={() => onRemove(subject)} style={iconButton}>
        <Icon name="trash" size={16} />
      </button>
    </li>
  );
}

const iconButton = {
  display: "grid",
  placeItems: "center",
  width: 44,
  height: 44,
  borderRadius: "var(--r-pill)",
  color: "var(--text-muted)",
  background: "transparent",
} as const;

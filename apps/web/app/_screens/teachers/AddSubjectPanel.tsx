"use client";

import { useMemo, useRef, useState, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { SelectField, useFocusFirstInvalid } from "../../_design/form";
import type { TeachingLoadItem } from "../../teachers/api";
import { groupChoices, isSelectable, subjectNames, type LoadCatalogue } from "../../teachers/load";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { GroupGrid } from "./GroupGrid";

export interface AddSubjectPanelProps {
  teacherId: string;
  catalogue: LoadCatalogue;
  /** Sends the picked slots; throws when the request itself failed. */
  onAssign: (items: TeachingLoadItem[]) => Promise<void>;
}

const NO_SUBJECT = "Elige una asignatura.";
const NO_GROUP = "Elige al menos un grupo.";

/**
 * Add a subject to the load: pick the subject BY NAME, then the groups. Inline
 * on the page and not a drawer — a school's group grid can be long, and a
 * drawer that scrolls is the sign it should have been a page.
 */
export function AddSubjectPanel({ teacherId, catalogue, onAssign }: AddSubjectPanelProps): ReactElement {
  const [subjectName, setSubjectName] = useState("");
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [subjectError, setSubjectError] = useState<string | undefined>(undefined);
  const [groupError, setGroupError] = useState<string | undefined>(undefined);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const gridRef = useRef<HTMLDivElement>(null);
  const focusFirstInvalid = useFocusFirstInvalid();

  const names = useMemo(() => subjectNames(catalogue.subjects), [catalogue.subjects]);
  const tree = useMemo(
    () => (subjectName === "" ? [] : groupChoices(teacherId, subjectName, catalogue)),
    [teacherId, subjectName, catalogue],
  );

  const chooseSubject = (name: string): void => {
    setSubjectName(name);
    setSelected(new Set());
    setGroupError(undefined);
  };

  const pickedItems = (): TeachingLoadItem[] =>
    tree.flatMap((l) => l.grades.flatMap((g) => g.groups)).flatMap(({ group, availability }) =>
      isSelectable(availability) && selected.has(group.id) ? [{ groupId: group.id, subjectId: availability.subjectId }] : [],
    );

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (pending) return;
    if (subjectName === "") {
      setSubjectError(NO_SUBJECT);
      focusFirstInvalid();
      return;
    }
    const items = pickedItems();
    if (items.length === 0) {
      setGroupError(NO_GROUP);
      gridRef.current?.querySelector<HTMLInputElement>("input:not(:disabled)")?.focus();
      return;
    }
    setPending(true);
    setFailure(undefined);
    try {
      await onAssign(items);
      setSelected(new Set());
    } catch {
      setFailure("No se pudo asignar. Revisa tu conexión e intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  const count = pickedItems().length;

  return (
    <Section title="Agregar asignatura" subtitle="Elige la asignatura y luego los grupos donde la imparte.">
      <form aria-label="Agregar asignatura" onSubmit={submit} style={{ display: "grid", gap: 14 }}>
        <SelectField
          label="Asignatura"
          value={subjectName}
          options={names.map((n) => ({ value: n, label: n }))}
          onChange={chooseSubject}
          error={subjectError}
          onClearError={() => setSubjectError(undefined)}
          help="Por nombre: cada grupo recibe la de su propio grado."
        />
        {subjectName !== "" && (
          <div ref={gridRef}>
            {tree.length === 0 ? <EmptyHint>Este ciclo aún no tiene grupos.</EmptyHint> : (
              <GroupGrid tree={tree} selected={selected} onChange={(next) => { setSelected(next); setGroupError(undefined); }} />
            )}
          </div>
        )}
        {groupError !== undefined && <ErrorAlert message={groupError} />}
        <div>
          <Btn type="submit" icon="plus">
            Asignar ({count} {count === 1 ? "grupo" : "grupos"})
          </Btn>
        </div>
      </form>
      {failure !== undefined && <ErrorAlert message={failure} />}
    </Section>
  );
}

"use client";

import { useMemo, useState, type KeyboardEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
import type { ScheduleBlock, ScheduleBoard } from "../../schedule/api";
import { blockLabel, type ScheduleIndex } from "../../schedule/catalog";
import { BlockFields, saveBlock, useBlockForm, type DrawerTarget } from "./blockForm";
import { OverrideFields, saveOverride, useOverrideForm } from "./overrideForm";
import { Tabs } from "./parts";

export type { DrawerTarget } from "./blockForm";

type Section = "class" | "override";

export interface BlockDrawerProps {
  target: DrawerTarget;
  board: ScheduleBoard;
  index: ScheduleIndex;
  grid: { start: number; end: number };
  /** The school's date (aaaa-mm-dd): where "the next Monday" is counted from. */
  today: string;
  onClose: () => void;
  onSaved: (message: string) => void;
  /** Remove the class (with the reason when the cycle has started); the caller offers the undo. */
  onRemove: (block: ScheduleBlock, reason: string) => void;
}

/**
 * Add a class, or open one: change it, remove it, or change it for ONE date
 * (a substitute, a swap, a cancellation) without touching the base week.
 * At most six fields per section → a dialog (forms rule). A server refusal
 * ("Ana Ruiz ya da Matemáticas a 3.º A…") is shown as is and the drawer
 * stays open.
 */
export function BlockDrawer({ target, board, index, grid, today, onClose, onSaved, onRemove }: BlockDrawerProps): ReactElement | null {
  const editing = target.kind === "edit" ? target.block : undefined;
  // A new class has no date-specific changes yet; the hook still needs a block.
  // Memoized: it feeds the "who is free" request's dependencies.
  const anchor = useMemo<ScheduleBlock>(
    () => editing ?? { id: "", cycleId: board.cycle.id, assignmentId: "", groupId: "", subjectId: "", teacherId: "", weekday: 1, startsAt: "", endsAt: "" },
    [editing, board.cycle.id],
  );
  const [section, setSection] = useState<Section>("class");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const blockForm = useBlockForm(target, board, grid.end);
  const overrideForm = useOverrideForm(anchor, board, today, section === "override");

  const submit = async (): Promise<void> => {
    if (pending) return;
    setError(undefined);
    setPending(true);
    try {
      const message = section === "class" ? await saveBlock(blockForm, target, board.started) : await saveOverride(overrideForm, anchor);
      if (message !== null) onSaved(message);
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo guardar.");
    } finally {
      setPending(false);
    }
  };

  const remove = (): void => {
    if (editing === undefined) return;
    // Once the cycle started, the removal needs its reason (the field is right there).
    if (board.started && blockForm.valid() === null) return;
    onRemove(editing, blockForm.values.reason.trim());
  };

  // Cmd/Ctrl+Enter saves from any field, selects included (forms rule).
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>): void => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      void submit();
    }
  };

  return (
    <SlideOver
      title={editing === undefined ? "Agregar clase" : index.subjectName(editing.subjectId)}
      subtitle={editing === undefined ? "Una de las asignaciones del grupo, en un día y una hora" : blockLabel(index, editing)}
      icon="clock"
      submitLabel={section === "class" ? "Guardar clase" : "Registrar cambio"}
      pending={pending}
      error={error}
      onClose={onClose}
      onSubmit={() => void submit()}
    >
      <div onKeyDown={onKeyDown} style={{ display: "grid", gap: 18, gridTemplateColumns: "minmax(0, 1fr)" }}>
        {editing !== undefined && (
          <Tabs label="Qué cambiar" active={section} onChoose={(s) => { setSection(s); setError(undefined); }}
            tabs={[{ id: "class", label: "Toda la semana" }, { id: "override", label: "Cambio solo para una fecha" }]} />
        )}
        {section === "class" ? (
          <>
            <BlockFields form={blockForm} board={board} index={index} grid={grid} />
            {editing !== undefined && (
              <div>
                <Btn kind="outline" icon="trash" onClick={remove} style={{ color: "var(--bad-fg)", borderColor: "var(--bad-fg)" }}>Quitar clase</Btn>
              </div>
            )}
          </>
        ) : (
          <OverrideFields form={overrideForm} block={anchor} board={board} index={index} />
        )}
      </div>
    </SlideOver>
  );
}

"use client";

import { useState, type ReactElement } from "react";

import { SlideOver } from "../../_design/SlideOver";
import type { ScheduleBlock, ScheduleBoard } from "../../schedule/api";
import type { ScheduleIndex } from "../../schedule/catalog";
import { spanishDate } from "../../schedule/time";
import { OverrideFields, saveOverride, useOverrideForm } from "./overrideForm";

export interface CoverDrawerProps {
  board: ScheduleBoard;
  index: ScheduleIndex;
  /** The class of the base timetable to cover. */
  block: ScheduleBlock;
  /** The date it is uncovered (aaaa-mm-dd). */
  date: string;
  /** Who is away: the reason is prefilled with it. */
  absentTeacherId: string;
  onClose: () => void;
  onSaved: (message: string) => void;
}

/**
 * "Asignar suplente" for one uncovered class on one date (T19): the T16
 * date-specific change with the date and the reason already filled, and the
 * free teachers (absent ones never listed) one choice away. Cancelling the
 * class that day covers it too.
 */
export function CoverDrawer({ board, index, block, date, absentTeacherId, onClose, onSaved }: CoverDrawerProps): ReactElement {
  const form = useOverrideForm(block, board, date, true, { reason: `Ausencia de ${index.teacherName(absentTeacherId)}` });
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const submit = async (): Promise<void> => {
    if (pending) return;
    setError(undefined);
    setPending(true);
    try {
      const message = await saveOverride(form, block);
      if (message !== null) onSaved(form.values.kind === "substitute" ? "Suplente asignado" : message);
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo guardar.");
    } finally {
      setPending(false);
    }
  };

  return (
    <SlideOver
      title="Asignar suplente"
      subtitle={`${index.subjectName(block.subjectId)} · ${index.groupLabel(block.groupId)} · ${spanishDate(date)}, ${block.startsAt}–${block.endsAt}`}
      icon="user"
      submitLabel={form.values.kind === "substitute" ? "Asignar suplente" : "Registrar cambio"}
      pending={pending}
      error={error}
      onClose={onClose}
      onSubmit={() => void submit()}
    >
      <OverrideFields form={form} block={block} board={board} index={index} />
    </SlideOver>
  );
}

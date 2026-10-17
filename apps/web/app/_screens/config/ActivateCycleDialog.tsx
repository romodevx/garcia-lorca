"use client";

import { useEffect, useId, useRef, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";

export interface ActivateCycleDialogProps {
  /** The year being put into service. */
  cycleName: string;
  /** The live year it stands down. */
  pausedName: string;
  onConfirm: () => void;
  onCancel: () => void;
}

/** The returnValue a confirmed close carries; anything else (Esc, Cancelar) is a cancel. */
const CONFIRMED = "confirm";

/**
 * "¿Activar el ciclo …?" — activating a year pauses the live one for the whole
 * school (QA CC-102: one click used to do it). A native modal <dialog>: the
 * browser gives top layer, inert background, Esc and focus back to the trigger.
 * The first focus is Cancelar, so a stray Enter does not pause the school.
 */
export function ActivateCycleDialog({ cycleName, pausedName, onConfirm, onCancel }: ActivateCycleDialogProps): ReactElement {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const titleId = useId();

  useEffect(() => {
    const dialog = dialogRef.current;
    if (dialog === null || dialog.open) return;
    dialog.showModal();
    cancelRef.current?.focus();
  }, []);

  const confirm = (e: FormEvent): void => {
    e.preventDefault();
    dialogRef.current?.close(CONFIRMED);
  };

  // Every way out (the two buttons, Esc) ends in "close"; returnValue says which.
  const closed = (): void => {
    if (dialogRef.current?.returnValue === CONFIRMED) onConfirm();
    else onCancel();
  };

  return (
    <dialog ref={dialogRef} className="gl-confirm" aria-labelledby={titleId} onClose={closed} style={dialogStyle}>
      <form onSubmit={confirm} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <h2 id={titleId} style={titleStyle}>¿Activar el ciclo {cycleName}?</h2>
        <p style={{ margin: 0, fontSize: 15, lineHeight: 1.55 }}>
          El ciclo {pausedName} queda en pausa para toda la escuela: grupos, colegiaturas e
          inscripciones pasan a {cycleName}.
        </p>
        <div style={{ display: "flex", gap: 12, justifyContent: "flex-end", flexWrap: "wrap" }}>
          <Btn kind="ghost" ref={cancelRef} onClick={() => dialogRef.current?.close()}>Cancelar</Btn>
          <Btn type="submit" icon="check">Activar {cycleName}</Btn>
        </div>
      </form>
    </dialog>
  );
}

const dialogStyle = {
  maxWidth: "min(480px, calc(100vw - 32px))",
  padding: 24,
  borderRadius: "var(--r-lg)",
  border: "1px solid var(--border)",
  background: "var(--surface)",
  color: "var(--text)",
} as const;

const titleStyle = {
  margin: 0,
  fontFamily: "var(--font-display)",
  fontSize: 20,
  fontWeight: 600,
} as const;

"use client";

import { useEffect, useId, useRef, useState, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { ErrorAlert } from "./parts";

export interface ActivateCycleDialogProps {
  /** The year being put into service. */
  cycleName: string;
  /** The live year it stands down. */
  pausedName: string;
  /** Activates. Rejects to keep the dialog open with `failureMessage` of the cause. */
  onConfirm: () => Promise<void>;
  /** The words for a refused activation (the server's, when it gave any). */
  failureMessage: (cause: unknown) => string;
  /** The dialog closed: activated, cancelled or Esc. */
  onClose: () => void;
}

/**
 * "¿Activar el ciclo …?" — a native modal <dialog>: the browser gives top
 * layer, inert background, Esc and focus back to the trigger.
 *
 * Why a confirmation for something reversible. The house rule keeps "¿Seguro?"
 * for what cannot be undone, and activating a year CAN be undone (activate the
 * other one again). This is the deliberate exception, not a precedent: it
 * pauses the live year for the WHOLE school — groups, tuition, enrolment all
 * switch at once, for every user — and one stray click used to do it (QA
 * CC-102). It is asked only when a live year would actually be paused. A
 * screen-local reversible action still gets an undo, not a dialog.
 *
 * The first focus is Cancelar, so a stray Enter does not pause the school. A
 * refusal (e.g. CYCLE_OVERLAP) is shown here and the dialog stays open.
 */
export function ActivateCycleDialog({ cycleName, pausedName, onConfirm, failureMessage, onClose }: ActivateCycleDialogProps): ReactElement {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const inFlight = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const titleId = useId();

  useEffect(() => {
    const dialog = dialogRef.current;
    if (dialog === null || dialog.open) return;
    dialog.showModal();
    cancelRef.current?.focus();
  }, []);

  // Never disabled while saving: it says it is busy and ignores the repeat click.
  const confirm = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError(undefined);
    try {
      await onConfirm();
      dialogRef.current?.close();
    } catch (cause) {
      setError(failureMessage(cause));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  };

  return (
    <dialog ref={dialogRef} className="gl-confirm" aria-labelledby={titleId} onClose={onClose} style={dialogStyle}>
      <form onSubmit={(e) => void confirm(e)} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <h2 id={titleId} style={titleStyle}>¿Activar el ciclo {cycleName}?</h2>
        <p style={{ margin: 0, fontSize: 15, lineHeight: 1.55 }}>
          El ciclo {pausedName} queda en pausa para toda la escuela: grupos, colegiaturas e
          inscripciones pasan a {cycleName}.
        </p>
        {error !== undefined && <ErrorAlert message={error} />}
        <div style={{ display: "flex", gap: 12, justifyContent: "flex-end", flexWrap: "wrap" }}>
          <Btn kind="ghost" ref={cancelRef} onClick={() => dialogRef.current?.close()}>Cancelar</Btn>
          <Btn type="submit" icon="check" busy={busy}>Activar {cycleName}</Btn>
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

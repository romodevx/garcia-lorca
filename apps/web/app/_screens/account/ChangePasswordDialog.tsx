"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import type { SessionEndReason } from "./changePasswordRules";
import { PasswordFields } from "./PasswordFields";
import { usePasswordChange } from "./usePasswordChange";

export interface ChangePasswordDialogProps {
  onSubmit: (currentPassword: string, newPassword: string) => Promise<{ sessionRenewed: boolean }>;
  onClose: () => void;
  /** No live session remains (see usePasswordChange). */
  onSessionEnded: (reason: SessionEndReason) => void;
}

const row = { display: "flex", alignItems: "center", gap: 12, width: "100%" } as const;

/**
 * Change your own password at will (ISSUE-T14-103). Three fields, so a small
 * dialog. Same code path as the forced screen (usePasswordChange).
 *
 * Esc, the backdrop and ✕ ask before discarding typed passwords, in the
 * dialog's own footer: never a second dialog on top. "Cancelar" is already
 * an explicit choice, so it closes.
 */
export function ChangePasswordDialog({ onSubmit, onClose, onSessionEnded }: ChangePasswordDialogProps): ReactElement {
  const [done, setDone] = useState(false);
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);
  const change = usePasswordChange({ onSubmit, onRenewed: () => setDone(true), onSessionEnded });
  const keepEditingRef = useRef<HTMLButtonElement>(null);
  const fieldsRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (confirmingDiscard) keepEditingRef.current?.focus();
  }, [confirmingDiscard]);

  const requestClose = (): void => {
    if (change.dirty && !done) setConfirmingDiscard(true);
    else onClose();
  };

  const keepEditing = (): void => {
    setConfirmingDiscard(false);
    fieldsRef.current?.querySelector("input")?.focus();
  };

  const footer = done ? (
    <div style={{ ...row, justifyContent: "flex-end" }}>
      <Btn onClick={onClose} icon="check">Listo</Btn>
    </div>
  ) : confirmingDiscard ? (
    <div style={row}>
      <span style={{ flex: 1, fontWeight: 600, color: "var(--text)" }}>¿Descartar lo que escribiste?</span>
      <Btn ref={keepEditingRef} kind="ghost" onClick={keepEditing}>Seguir editando</Btn>
      <Btn danger onClick={onClose}>Descartar</Btn>
    </div>
  ) : (
    <div style={row}>
      <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
      <div style={{ flex: 1 }} />
      <Btn type="submit" icon="shield" busy={change.saving}>Cambiar contraseña</Btn>
    </div>
  );

  return (
    <DrawerShell
      title="Cambiar contraseña"
      icon="shield"
      size="sm"
      onClose={requestClose}
      onSubmit={() => { if (!done && !confirmingDiscard) void change.submit(); }}
      footer={footer}
    >
      {done ? (
        <p role="status" style={{ fontSize: 15, lineHeight: 1.55, color: "var(--text)" }}>
          Tu contraseña cambió. Las demás sesiones dejarán de funcionar en unos minutos.
        </p>
      ) : (
        <div ref={fieldsRef}>
          <PasswordFields change={change} />
        </div>
      )}
    </DrawerShell>
  );
}

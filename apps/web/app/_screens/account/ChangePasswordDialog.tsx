"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { PasswordFields } from "./PasswordFields";
import { usePasswordChange } from "./usePasswordChange";

export interface ChangePasswordDialogProps {
  onSubmit: (currentPassword: string, newPassword: string) => Promise<{ sessionRenewed: boolean }>;
  onClose: () => void;
  /** The password changed but the server could not hand back a fresh session. */
  onSessionEnded: () => void;
}

/**
 * Change your own password at will (ISSUE-T14-103). Three fields, so a small
 * dialog. Same code path as the forced screen (usePasswordChange).
 */
export function ChangePasswordDialog({ onSubmit, onClose, onSessionEnded }: ChangePasswordDialogProps): ReactElement {
  const [done, setDone] = useState(false);
  const change = usePasswordChange({ onSubmit, onRenewed: () => setDone(true), onSessionEnded });

  const footer = done ? (
    <div style={{ display: "flex", width: "100%", justifyContent: "flex-end" }}>
      <Btn onClick={onClose} icon="check">Listo</Btn>
    </div>
  ) : (
    <div style={{ display: "flex", alignItems: "center", gap: 12, width: "100%" }}>
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
      onClose={onClose}
      onSubmit={() => { if (!done) void change.submit(); }}
      footer={footer}
    >
      {done ? (
        <p role="status" style={{ fontSize: 15, lineHeight: 1.55, color: "var(--text)" }}>
          Tu contraseña cambió. Cerramos las demás sesiones abiertas con la anterior; aquí sigues dentro.
        </p>
      ) : (
        <PasswordFields change={change} />
      )}
    </DrawerShell>
  );
}

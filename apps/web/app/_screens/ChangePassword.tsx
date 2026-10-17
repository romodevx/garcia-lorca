"use client";

import type { ReactElement } from "react";

import { Btn, ThemeToggle } from "../_design/Controls";
import type { Theme } from "../theme/theme-script";
import type { SessionEndReason } from "./account/changePasswordRules";
import { PasswordFields } from "./account/PasswordFields";
import { usePasswordChange } from "./account/usePasswordChange";

export interface ChangePasswordProps {
  theme: Theme;
  onToggle: () => void;
  onSubmit: (currentPassword: string, newPassword: string) => Promise<{ sessionRenewed: boolean }>;
  /** The password changed and the session was renewed: go on into the system. */
  onDone: () => void;
  /** No live session remains (see usePasswordChange). */
  onSessionEnded: (reason: SessionEndReason) => void;
}

/**
 * The FORCED change after signing in with a temporary password. Same code path
 * as the at-will dialog (usePasswordChange): the primary is never disabled,
 * every error is reported on submit, and a 204 hands over to sign-in.
 */
export function ChangePassword({ theme, onToggle, onSubmit, onDone, onSessionEnded }: ChangePasswordProps): ReactElement {
  const change = usePasswordChange({ onSubmit, onRenewed: onDone, onSessionEnded });

  return (
    <div className="app-root" style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: 40 }}>
      <div style={{ position: "absolute", top: 24, right: 28 }}>
        <ThemeToggle theme={theme} onToggle={onToggle} />
      </div>
      <form
        className="fade-up"
        noValidate
        onSubmit={(e) => { e.preventDefault(); void change.submit(); }}
        style={{ width: "100%", maxWidth: 412 }}
      >
        <h2 style={{ fontSize: 28, fontWeight: 700, color: "var(--text)" }}>Crea tu contraseña</h2>
        <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 8, marginBottom: 22 }}>
          Por seguridad, cambia la contraseña temporal antes de continuar.
        </p>
        <PasswordFields change={change} currentLabel="Contraseña temporal" />
        <Btn type="submit" full icon="shield" busy={change.saving} style={{ height: 54, fontSize: 17, marginTop: 20 }}>
          Guardar y entrar
        </Btn>
      </form>
    </div>
  );
}

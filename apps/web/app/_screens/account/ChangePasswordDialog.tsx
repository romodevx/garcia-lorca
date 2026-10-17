"use client";

import { useRef, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { Field, inputStyle } from "../../_design/form";
import {
  FIELD_ORDER,
  describeChangeFailure,
  validatePasswordDraft,
  type PasswordDraft,
  type PasswordErrors,
  type PasswordField,
} from "./changePasswordRules";

export interface ChangePasswordDialogProps {
  onSubmit: (currentPassword: string, newPassword: string) => Promise<{ sessionRenewed: boolean }>;
  onClose: () => void;
  /** The password changed but the server could not hand back a fresh session. */
  onSessionEnded: () => void;
}

const LABELS: Record<PasswordField, string> = {
  current: "Contraseña actual",
  next: "Nueva contraseña",
  confirm: "Confirmar contraseña",
};

const AUTOCOMPLETE: Record<PasswordField, string> = {
  current: "current-password",
  next: "new-password",
  confirm: "new-password",
};

const EMPTY: PasswordDraft = { current: "", next: "", confirm: "" };

/**
 * Change your own password at will (ISSUE-T14-103). Three fields, so a small
 * dialog. The primary is never disabled: a repeat click while saving is
 * ignored, and every error is reported on submit, with focus on its field.
 */
export function ChangePasswordDialog({ onSubmit, onClose, onSessionEnded }: ChangePasswordDialogProps): ReactElement {
  const [draft, setDraft] = useState<PasswordDraft>(EMPTY);
  const [errors, setErrors] = useState<PasswordErrors>({});
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [done, setDone] = useState(false);
  const inputs = useRef<Partial<Record<PasswordField, HTMLInputElement | null>>>({});
  const inFlight = useRef(false);

  const focusFirst = (found: PasswordErrors): void => {
    const first = FIELD_ORDER.find((field) => found[field] !== undefined);
    if (first !== undefined) inputs.current[first]?.focus();
  };

  const submit = async (): Promise<void> => {
    if (inFlight.current || done) return;
    const found = validatePasswordDraft(draft);
    setErrors(found);
    setFailure(undefined);
    if (Object.keys(found).length > 0) {
      focusFirst(found);
      return;
    }
    inFlight.current = true;
    setSaving(true);
    try {
      const { sessionRenewed } = await onSubmit(draft.current, draft.next);
      if (!sessionRenewed) {
        onSessionEnded();
        return;
      }
      setDone(true);
    } catch (err) {
      const { field, message } = describeChangeFailure(err);
      if (field === undefined) {
        setFailure(message);
      } else {
        const onField = { [field]: message };
        setErrors(onField);
        focusFirst(onField);
      }
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
  };

  const footer = done ? (
    <div style={{ display: "flex", width: "100%", justifyContent: "flex-end" }}>
      <Btn onClick={onClose} icon="check">Listo</Btn>
    </div>
  ) : (
    <div style={{ display: "flex", alignItems: "center", gap: 12, width: "100%" }}>
      <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
      <div style={{ flex: 1 }} />
      <Btn type="submit" icon="shield" busy={saving}>{saving ? "Cambiando contraseña…" : "Cambiar contraseña"}</Btn>
    </div>
  );

  return (
    <DrawerShell title="Cambiar contraseña" icon="shield" size="sm" onClose={onClose} onSubmit={() => void submit()} footer={footer}>
      {done ? (
        <p role="status" style={{ fontSize: 15, lineHeight: 1.55, color: "var(--text)" }}>
          Tu contraseña cambió. Cerramos las demás sesiones abiertas con la anterior; aquí sigues dentro.
        </p>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          {FIELD_ORDER.map((field) => (
            <Field key={field} label={LABELS[field]} required error={errors[field]}>
              {(id, describedBy) => (
                <input
                  id={id}
                  ref={(node) => { inputs.current[field] = node; }}
                  type="password"
                  autoComplete={AUTOCOMPLETE[field]}
                  aria-invalid={errors[field] !== undefined ? true : undefined}
                  aria-describedby={describedBy}
                  value={draft[field]}
                  onChange={(e) => setDraft((d) => ({ ...d, [field]: e.target.value }))}
                  style={{ ...inputStyle(errors[field] !== undefined), fontSize: 16 }}
                />
              )}
            </Field>
          ))}
          {failure !== undefined && (
            <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, fontWeight: 600 }}>{failure}</p>
          )}
        </div>
      )}
    </DrawerShell>
  );
}

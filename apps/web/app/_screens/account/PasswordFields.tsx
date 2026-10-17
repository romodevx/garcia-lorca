"use client";

import type { ReactElement } from "react";

import { Field, inputStyle } from "../../_design/form";
import { FIELD_ORDER, type PasswordField } from "./changePasswordRules";
import type { PasswordChange } from "./usePasswordChange";

const DEFAULT_LABELS: Record<PasswordField, string> = {
  current: "Contraseña actual",
  next: "Nueva contraseña",
  confirm: "Confirmar contraseña",
};

const AUTOCOMPLETE: Record<PasswordField, string> = {
  current: "current-password",
  next: "new-password",
  confirm: "new-password",
};

export interface PasswordFieldsProps {
  change: PasswordChange;
  /** The forced screen calls the current one "Contraseña temporal". */
  currentLabel?: string;
}

/** The three fields and the failure line, one column, for both surfaces. */
export function PasswordFields({ change, currentLabel }: PasswordFieldsProps): ReactElement {
  const labels = { ...DEFAULT_LABELS, ...(currentLabel === undefined ? {} : { current: currentLabel }) };
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      {FIELD_ORDER.map((field) => {
        const error = change.errors[field];
        return (
          <Field key={field} label={labels[field]} required error={error}>
            {(id, describedBy) => (
              <input
                id={id}
                ref={change.bindInput(field)}
                type="password"
                autoComplete={AUTOCOMPLETE[field]}
                aria-invalid={error !== undefined ? true : undefined}
                aria-describedby={describedBy}
                value={change.draft[field]}
                onChange={(e) => change.setField(field, e.target.value)}
                style={{ ...inputStyle(error !== undefined), fontSize: 16 }}
              />
            )}
          </Field>
        );
      })}
      {change.failure !== undefined && (
        <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, fontWeight: 600 }}>{change.failure}</p>
      )}
    </div>
  );
}

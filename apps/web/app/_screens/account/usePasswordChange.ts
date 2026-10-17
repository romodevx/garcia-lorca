"use client";

import { useRef, useState } from "react";

import {
  FIELD_ORDER,
  describeChangeFailure,
  validatePasswordDraft,
  type PasswordDraft,
  type PasswordErrors,
  type PasswordField,
} from "./changePasswordRules";

export interface PasswordChangeOptions {
  onSubmit: (currentPassword: string, newPassword: string) => Promise<{ sessionRenewed: boolean }>;
  /** The password changed and the gateway handed back a fresh session (200). */
  onRenewed: () => void;
  /** The password changed but the session could not be renewed (204). */
  onSessionEnded: () => void;
}

export interface PasswordChange {
  draft: PasswordDraft;
  errors: PasswordErrors;
  failure?: string;
  saving: boolean;
  setField: (field: PasswordField, value: string) => void;
  bindInput: (field: PasswordField) => (node: HTMLInputElement | null) => void;
  submit: () => Promise<void>;
}

const EMPTY: PasswordDraft = { current: "", next: "", confirm: "" };

/**
 * The ONE change-password code path, shared by the forced screen (temporary
 * password) and the at-will dialog. The primary is never disabled: a click
 * while a change is in flight is ignored here instead. Errors are reported on
 * submit, with focus on the first field that needs attention.
 */
export function usePasswordChange({ onSubmit, onRenewed, onSessionEnded }: PasswordChangeOptions): PasswordChange {
  const [draft, setDraft] = useState<PasswordDraft>(EMPTY);
  const [errors, setErrors] = useState<PasswordErrors>({});
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const inputs = useRef<Partial<Record<PasswordField, HTMLInputElement | null>>>({});
  const inFlight = useRef(false);

  const report = (found: PasswordErrors): void => {
    setErrors(found);
    const first = FIELD_ORDER.find((field) => found[field] !== undefined);
    if (first !== undefined) inputs.current[first]?.focus();
  };

  const submit = async (): Promise<void> => {
    if (inFlight.current) return;
    const found = validatePasswordDraft(draft);
    setFailure(undefined);
    report(found);
    if (Object.keys(found).length > 0) return;
    inFlight.current = true;
    setSaving(true);
    try {
      const { sessionRenewed } = await onSubmit(draft.current, draft.next);
      if (sessionRenewed) onRenewed();
      else onSessionEnded();
    } catch (err) {
      const { field, message } = describeChangeFailure(err);
      if (field === undefined) setFailure(message);
      else report({ [field]: message });
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
  };

  return {
    draft,
    errors,
    failure,
    saving,
    setField: (field, value) => setDraft((d) => ({ ...d, [field]: value })),
    bindInput: (field) => (node) => { inputs.current[field] = node; },
    submit,
  };
}

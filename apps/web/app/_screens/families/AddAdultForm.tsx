"use client";

import { useEffect, useId, useState, type FormEvent, type KeyboardEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { filterChip } from "../../_design/students/StudentFilters";
import { EmailField, TextField, useEmailShapeOnSave, useFocusFirstInvalid, useFormatError, useRequired } from "../../_design/form";
import { API_URL } from "../../api-url";
import { addFamilyContact, familyRefusal } from "../../families/api";
import type { Relationship } from "../../students/api";
import { phoneDigits, phoneError } from "../../students/profileRules";
import { RELATIONSHIPS } from "../../students/relationship";
import { DiscardPrompt } from "./DiscardPrompt";

const NAME_REQUIRED = ["name"] as const;
const PHONE_RULES = { phone: phoneError } as const;
const CHOOSE_RELATIONSHIP = "Elige el parentesco";

export interface AddAdultFormProps {
  familyId: string;
  onAdded: (name: string) => void;
  onCancel: () => void;
  /** Whether anything was typed or chosen: the drawer asks before closing over it. */
  onDirtyChange: (dirty: boolean) => void;
}

/**
 * One adult, written on every child of the family at once (five fields in
 * the drawer it already sits in). Errors appear on save; the button is never
 * dead. Esc is the form's own: untouched it closes the form, half-written it
 * asks — it never reaches the drawer, which would throw the adult away.
 */
export function AddAdultForm({ familyId, onAdded, onCancel, onDirtyChange }: AddAdultFormProps): ReactElement {
  const errorId = useId();
  const [relationship, setRelationship] = useState<Relationship | undefined>(undefined);
  const [relationshipMissing, setRelationshipMissing] = useState(false);
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [email, setEmail] = useState("");
  const [emergency, setEmergency] = useState(false);
  const [saving, setSaving] = useState(false);
  const [asking, setAsking] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const required = useRequired(NAME_REQUIRED);
  const emailShape = useEmailShapeOnSave();
  const phoneShape = useFormatError(PHONE_RULES);
  const focusFirstInvalid = useFocusFirstInvalid();
  const dirty = relationship !== undefined || emergency || [name, phone, email].some((v) => v.trim() !== "");
  useEffect(() => onDirtyChange(dirty), [dirty, onDirtyChange]);

  const save = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    const named = required.check({ name });
    const shaped = emailShape.check(email);
    const phoned = phoneShape.checkAll({ phone });
    setRelationshipMissing(relationship === undefined);
    if (relationship === undefined) focusFirstInvalid();
    if (!named || !shaped || !phoned || relationship === undefined || saving) return;
    setSaving(true);
    setError(undefined);
    try {
      await addFamilyContact(API_URL, familyId, { relationship, fullName: name, phone: phone.trim() === "" ? "" : phoneDigits(phone), email, isEmergency: emergency });
      onAdded(name.trim());
    } catch (cause) {
      setError(familyRefusal(cause, "No se pudo agregar al adulto."));
      setSaving(false);
    }
  };
  const onKeyDown = (e: KeyboardEvent): void => {
    if (e.key !== "Escape") return;
    e.stopPropagation();
    if (dirty) setAsking(true);
    else onCancel();
  };

  return (
    <form onSubmit={(e) => void save(e)} onKeyDown={onKeyDown} noValidate style={{ display: "grid", gap: 12, padding: "14px 16px", borderRadius: "var(--r-lg)", background: "var(--surface-2)", border: "1px solid var(--border)" }}>
      <div role="group" aria-label="Parentesco" style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {RELATIONSHIPS.map((c, i) => (
          <button
            key={c.value}
            type="button"
            aria-pressed={relationship === c.value}
            aria-invalid={relationshipMissing && i === 0 ? true : undefined}
            aria-describedby={relationshipMissing ? errorId : undefined}
            onClick={() => { setRelationship(c.value); setRelationshipMissing(false); }}
            style={filterChip(relationship === c.value)}
          >
            {c.label}
          </button>
        ))}
      </div>
      {relationshipMissing && <p id={errorId} role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{CHOOSE_RELATIONSHIP}</p>}
      <TextField label="Nombre del adulto" value={name} onChange={setName} uppercase required error={required.errors.name} onClearError={() => required.clear("name")} />
      <TextField
        label="Teléfono del adulto"
        value={phone}
        onChange={(v) => { setPhone(v); phoneShape.onChange("phone", v, {}); }}
        onBlur={() => phoneShape.onBlur("phone", phone, {})}
        error={phoneShape.errors.phone}
        help="A diez dígitos. Puede quedar vacío."
      />
      <EmailField label="Correo del adulto" value={email} onChange={(v) => { setEmail(v); emailShape.forgive(v); }} error={emailShape.error} help="Hace falta para darle acceso a la app. Puede quedar vacío." />
      <label style={{ display: "inline-flex", alignItems: "center", gap: 8, minHeight: 44, fontSize: 14, color: "var(--text)", cursor: "pointer" }}>
        <input type="checkbox" checked={emergency} onChange={(e) => setEmergency(e.target.checked)} style={{ width: 20, height: 20, accentColor: "var(--forest)" }} />
        Contacto de emergencia
      </label>
      {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{error}</p>}
      {asking ? (
        <DiscardPrompt question="¿Descartar a este adulto?" onDiscard={onCancel} onKeep={() => setAsking(false)} />
      ) : (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <Btn type="submit" icon="check" busy={saving}>Guardar adulto</Btn>
          <Btn kind="ghost" onClick={onCancel}>Cancelar</Btn>
        </div>
      )}
    </form>
  );
}

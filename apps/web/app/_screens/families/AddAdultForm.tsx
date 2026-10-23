"use client";

import { useState, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { filterChip } from "../../_design/students/StudentFilters";
import { EmailField, TextField, useEmailShapeOnSave, useFormatError, useRequired } from "../../_design/form";
import { API_URL } from "../../api-url";
import { addFamilyContact, familyRefusal } from "../../families/api";
import type { Relationship } from "../../students/api";
import { phoneDigits, phoneError } from "../../students/profileRules";

const NAME_REQUIRED = ["name"] as const;
const PHONE_RULES = { phone: phoneError } as const;
const CHOOSE_RELATIONSHIP = "Elige el parentesco";

/** Spoken the way the office asks at the window. */
const CHOICES: readonly { value: Relationship; label: string }[] = [
  { value: "mother", label: "Mamá" },
  { value: "father", label: "Papá" },
  { value: "guardian", label: "Tutor" },
  { value: "other", label: "Otro" },
];

/**
 * One adult, written on every child of the family at once (five fields in
 * the drawer it already sits in). Errors appear on save; the button is never
 * dead.
 */
export function AddAdultForm({ familyId, onAdded, onCancel }: { familyId: string; onAdded: (name: string) => void; onCancel: () => void }): ReactElement {
  const [relationship, setRelationship] = useState<Relationship | undefined>(undefined);
  const [relationshipMissing, setRelationshipMissing] = useState(false);
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [email, setEmail] = useState("");
  const [emergency, setEmergency] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const required = useRequired(NAME_REQUIRED);
  const emailShape = useEmailShapeOnSave();
  const phoneShape = useFormatError(PHONE_RULES);

  const save = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    const named = required.check({ name });
    const shaped = emailShape.check(email);
    const phoned = phoneShape.checkAll({ phone });
    setRelationshipMissing(relationship === undefined);
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

  return (
    <form onSubmit={(e) => void save(e)} noValidate style={{ display: "grid", gap: 12, padding: "14px 16px", borderRadius: "var(--r-lg)", background: "var(--surface-2)", border: "1px solid var(--border)" }}>
      <div role="group" aria-label="Parentesco" style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {CHOICES.map((c) => (
          <button key={c.value} type="button" aria-pressed={relationship === c.value} onClick={() => { setRelationship(c.value); setRelationshipMissing(false); }} style={filterChip(relationship === c.value)}>{c.label}</button>
        ))}
      </div>
      {relationshipMissing && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{CHOOSE_RELATIONSHIP}</p>}
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
        <input type="checkbox" checked={emergency} onChange={(e) => setEmergency(e.target.checked)} aria-label="Contacto de emergencia" style={{ width: 20, height: 20, accentColor: "var(--forest)" }} />
        De emergencia
      </label>
      {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{error}</p>}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <Btn type="submit" icon="check" busy={saving}>Guardar adulto</Btn>
        <Btn kind="ghost" onClick={onCancel}>Cancelar</Btn>
      </div>
    </form>
  );
}

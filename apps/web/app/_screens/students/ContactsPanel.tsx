"use client";

import { useCallback, useEffect, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { EmailField, SelectField, TextField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { addContact, listContacts, type Contact, type Relationship } from "../../students/api";
import { API_URL } from "../../api-url";

const RELATIONSHIPS: { value: Relationship; label: string }[] = [
  { value: "mother", label: "Madre" },
  { value: "father", label: "Padre" },
  { value: "guardian", label: "Tutor" },
  { value: "other", label: "Otro" },
];

const RELATIONSHIP_LABEL: Record<string, string> = {
  mother: "Madre", father: "Padre", guardian: "Tutor", other: "Otro",
};

export interface ContactsPanelProps {
  studentId: string;
}

/**
 * Who the school can call about this pupil.
 *
 * Most of them have no account and never will: a grandmother who may collect
 * the child, the father whose phone actually answers. Recording them used to
 * require creating a login, so the office kept the second number on paper and
 * the system showed one contact per pupil as though that were the truth.
 */
export function ContactsPanel({ studentId }: ContactsPanelProps): ReactElement {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [email, setEmail] = useState("");
  const [relationship, setRelationship] = useState<Relationship>("other");
  const [error, setError] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);

  const reload = useCallback(() => {
    listContacts(API_URL, studentId)
      .then(setContacts)
      .catch(() => setError("No pudimos leer los contactos."));
  }, [studentId]);

  useEffect(reload, [reload]);

  const add = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (name.trim() === "") {
      setError("Un contacto necesita nombre.");
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      await addContact(API_URL, {
        studentId,
        relationship,
        fullName: name.trim(),
        ...(phone.trim() !== "" ? { phone: phone.trim() } : {}),
        ...(email.trim() !== "" ? { email: email.trim() } : {}),
      });
      setName("");
      setPhone("");
      setEmail("");
      reload();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo agregar el contacto.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div style={{ padding: "18px 0" }}>
      <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Contactos</span>

      {contacts.length === 0 && (
        <p style={{ fontSize: 14, color: "var(--text-faint)", margin: "10px 0 0" }}>
          Todavía no hay contactos registrados.
        </p>
      )}
      {contacts.map((c) => (
        <div key={c.id} style={{ display: "flex", alignItems: "center", gap: 10, padding: "11px 0", borderBottom: "1px solid var(--border)" }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 14.5, fontWeight: 600, color: "var(--text)" }}>
              {c.fullName !== "" ? c.fullName : "Cuenta vinculada"}
            </div>
            <div style={{ fontSize: 13, color: "var(--text-muted)" }}>
              {RELATIONSHIP_LABEL[c.relationship] ?? c.relationship}
              {c.phone !== "" && ` · ${c.phone}`}
              {c.email !== "" && ` · ${c.email}`}
            </div>
          </div>
          {c.hasAccount && (
            // The one thing the office needs to tell apart: who can see the app,
            // and who is only a number to call.
            <span title="Puede entrar a la app" style={{ display: "inline-flex", alignItems: "center", gap: 5, padding: "4px 10px", borderRadius: 99, background: "var(--ok-bg)", color: "var(--ok-fg)", fontSize: 12, fontWeight: 700 }}>
              <Icon name="check" size={13} stroke={2.6} />App
            </span>
          )}
        </div>
      ))}

      <form onSubmit={(e) => void add(e)} style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 14 }}>
        <div style={{ flex: "2 1 150px", minWidth: 0 }}>
          <TextField label="Nombre del contacto" value={name} onChange={setName} uppercase required placeholder="NOMBRE" />
        </div>
        <div style={{ flex: "1 1 120px", minWidth: 0 }}>
          <TextField label="Teléfono del contacto" value={phone} onChange={setPhone} placeholder="Teléfono" />
        </div>
        <div style={{ flex: "1 1 150px", minWidth: 0 }}>
          <EmailField label="Correo del contacto" value={email} onChange={setEmail} placeholder="correo@ejemplo.mx" />
        </div>
        <div style={{ flex: "1 1 110px", minWidth: 0 }}>
          <SelectField
            label="Parentesco"
            value={relationship}
            onChange={(v) => setRelationship(v as Relationship)}
            options={RELATIONSHIPS.map((r) => ({ value: r.value, label: r.label }))}
          />
        </div>
        <Btn type="submit" icon="plus" style={{ opacity: saving ? 0.5 : 1 }}>Agregar</Btn>
      </form>
      {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: "10px 0 0" }}>{error}</p>}
    </div>
  );
}

"use client";

import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { EmailField, SelectField, TextField, useEmailShapeOnSave, useFormatError, useRequired } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { InviteLink } from "../../_design/InviteLink";
import { UndoToast } from "../../_design/UndoToast";
import { useUndoableRemoval } from "../../_design/useUndoableRemoval";
import { spanishServerReason } from "../../_lib/serverReason";
import {
  addContact,
  inviteContactToApp,
  listContacts,
  removeContact,
  updateContact,
  type Contact,
  type ContactAccessResult,
  type ContactEdit,
  type Relationship,
} from "../../students/api";
import { phoneDigits, phoneError } from "../../students/profileRules";
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
/** Fuera del componente: useRequired lo toma como dependencia. */
const CONTACT_REQUIRED = ["name"] as const;
/** A contact with an account may go without a name (the server allows it). */
const NOTHING_REQUIRED = [] as const;
/** Fuera del componente: useFormatError lo toma como dependencia. */
const PHONE_RULES = { phone: phoneError } as const;

/** iam's refusal of a staff address not yet confirmed (C2-003). */
const STAFF_ADDRESS_REASON = "belongs to a staff account";

export function ContactsPanel({ studentId }: ContactsPanelProps): ReactElement {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [email, setEmail] = useState("");
  const [relationship, setRelationship] = useState<Relationship>("other");
  const [emergency, setEmergency] = useState(false);
  const phoneShape = useFormatError(PHONE_RULES);
  const [editing, setEditing] = useState<string | undefined>(undefined);
  const removal = useUndoableRemoval<Contact>();
  // A reload inside the grace period must not bring a removed row back.
  const pendingRemoval = useRef<readonly Contact[]>([]);
  pendingRemoval.current = removal.pending;
  const [error, setError] = useState<string | undefined>(undefined);
  const form = useRequired(CONTACT_REQUIRED);
  // El correo puede quedar vacío, pero si se escribe tiene que tener forma de
  // correo: se revisa aquí, al guardar, porque Enter guarda sin salir del campo.
  const emailShape = useEmailShapeOnSave();
  const [saving, setSaving] = useState(false);
  // Which contact is being given access, and what came of it. Keyed by contact
  // so a second press elsewhere never shows the first one's outcome.
  const [granting, setGranting] = useState<string | undefined>(undefined);
  const [granted, setGranted] = useState<Record<string, ContactAccessResult>>({});
  const [grantError, setGrantError] = useState<Record<string, string>>({});
  // C2-003: contacts whose address iam said belongs to a staff account, waiting
  // for the director to say whether it is the same person.
  const [staffAddress, setStaffAddress] = useState<Record<string, boolean>>({});

  const reload = useCallback(() => {
    listContacts(API_URL, studentId)
      .then((rows) => setContacts(rows.filter((r) => !pendingRemoval.current.some((p) => p.id === r.id))))
      .catch(() => setError("No pudimos leer los contactos."));
  }, [studentId]);

  useEffect(reload, [reload]);

  const add = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    // Se marca EL CAMPO, no una frase al pie. El asterisco promete que el rojo
    // va a aparecer donde falta, y un mensaje abajo obliga a buscar cuál de los
    // cuatro campos era.
    // Ambas revisiones corren siempre, para marcar a la vez todo lo que falla.
    const complete = form.check({ name });
    const shaped = emailShape.check(email);
    const phoned = phoneShape.checkAll({ phone });
    if (!complete || !shaped || !phoned) return;
    setSaving(true);
    setError(undefined);
    try {
      await addContact(API_URL, {
        studentId,
        relationship,
        fullName: name.trim(),
        ...(phone.trim() !== "" ? { phone: phoneDigits(phone) } : {}),
        ...(email.trim() !== "" ? { email: email.trim() } : {}),
        ...(emergency ? { isEmergency: true } : {}),
      });
      setName("");
      setPhone("");
      setEmail("");
      setEmergency(false);
      reload();
    } catch (cause) {
      setError((cause instanceof Error ? spanishServerReason(cause.message) : undefined) ?? "No se pudo agregar el contacto.");
    } finally {
      setSaving(false);
    }
  };

  const saveEdit = async (contact: Contact, edit: ContactEdit): Promise<void> => {
    const saved = await updateContact(API_URL, studentId, contact.id, edit);
    setContacts((prev) => prev.map((c) => (c.id === saved.id ? saved : c)));
    setEditing(undefined);
  };

  const remove = (contact: Contact): void => {
    setContacts((prev) => prev.filter((c) => c.id !== contact.id));
    removal.remove(contact, ({ keepalive }) => {
      removeContact(API_URL, studentId, contact.id, { keepalive }).catch(() => {
        setError(`No se pudo quitar a ${contact.fullName}. Vuelve a cargar la página.`);
        reload();
      });
    });
  };

  const giveAccess = async (contact: Contact, confirmStaffAccount = false): Promise<void> => {
    setGranting(contact.id);
    setGrantError((prev) => ({ ...prev, [contact.id]: "" }));
    setStaffAddress((prev) => ({ ...prev, [contact.id]: false }));
    try {
      const result = await inviteContactToApp(API_URL, studentId, contact.id, confirmStaffAccount ? { confirmStaffAccount: true } : {});
      setGranted((prev) => ({ ...prev, [contact.id]: result }));
      reload();
    } catch (cause) {
      if (cause instanceof Error && cause.message.includes(STAFF_ADDRESS_REASON)) {
        setStaffAddress((prev) => ({ ...prev, [contact.id]: true }));
        return;
      }
      setGrantError((prev) => ({
        ...prev,
        [contact.id]: (cause instanceof Error ? spanishServerReason(cause.message) : undefined) ?? "No se pudo dar el acceso.",
      }));
    } finally {
      setGranting(undefined);
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
      {contacts.map((c) => editing === c.id ? (
        <ContactEditRow key={c.id} contact={c} onSave={(edit) => saveEdit(c, edit)} onCancel={() => setEditing(undefined)} />
      ) : (
        <div key={c.id} style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap", padding: "11px 0", borderBottom: "1px solid var(--border)" }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 14.5, fontWeight: 600, color: "var(--text)" }}>
              {displayName(c)}
            </div>
            <div style={{ fontSize: 13, color: "var(--text-muted)" }}>
              {RELATIONSHIP_LABEL[c.relationship] ?? c.relationship}
              {c.phone !== "" && ` · ${c.phone}`}
              {c.email !== "" && ` · ${c.email}`}
            </div>
          </div>
          {c.isEmergency === true && (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 5, padding: "4px 10px", borderRadius: 99, background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 12, fontWeight: 700 }}>
              <Icon name="phone" size={13} stroke={2.6} />Emergencias
            </span>
          )}
          {c.hasAccount && (
            // The one thing the office needs to tell apart: who can see the app,
            // and who is only a number to call.
            <span title="Puede entrar a la app" style={{ display: "inline-flex", alignItems: "center", gap: 5, padding: "4px 10px", borderRadius: 99, background: "var(--ok-bg)", color: "var(--ok-fg)", fontSize: 12, fontWeight: 700 }}>
              <Icon name="check" size={13} stroke={2.6} />App
            </span>
          )}
          {!c.hasAccount && c.email !== "" && (
            <Btn
              kind="ghost"
              icon="mail"
              onClick={() => void giveAccess(c)}
              disabled={granting === c.id}
              style={{ height: 36, padding: "0 14px", fontSize: 13.5, gap: 7 }}
            >
              {granting === c.id ? "Enviando…" : "Dar acceso a la app"}
            </Btn>
          )}
          {!c.hasAccount && c.email === "" && (
            // An action that cannot succeed should not be pressable. Saying WHY
            // is what turns a missing button into something the office can fix.
            <span style={{ fontSize: 12.5, color: "var(--text-faint)" }}>
              Falta el correo para invitarle
            </span>
          )}
          <Btn kind="ghost" onClick={() => setEditing(c.id)} aria-label={`Editar a ${displayName(c)}`} style={ROW_ACTION}>Editar</Btn>
          {!c.hasAccount && (
            // A contact who can open the app is refused by the server (409):
            // its access is revoked first. So the button is not offered.
            <Btn kind="ghost" onClick={() => remove(c)} aria-label={`Quitar a ${displayName(c)}`} style={ROW_ACTION}>Quitar</Btn>
          )}
        </div>
      ))}
      {contacts.map((c) => staffAddress[c.id] === true && (
        <div key={`${c.id}-staff`} role="status" style={{ margin: "8px 0 0", padding: "10px 12px", borderRadius: 10, background: "var(--surface-2)", fontSize: 13.5, color: "var(--text)", display: "grid", gap: 8 }}>
          <span>
            <strong>{c.email}</strong> es la cuenta de alguien del personal. Si es la misma persona —por ejemplo,
            un maestro que también es papá o mamá—, usará esa misma cuenta para entrar a la app. Si no, corrige el correo del contacto.
          </span>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <Btn onClick={() => void giveAccess(c, true)}>Sí, es la misma persona</Btn>
            <Btn kind="ghost" onClick={() => { setStaffAddress((prev) => ({ ...prev, [c.id]: false })); setEditing(c.id); }}>No, corregir el correo</Btn>
          </div>
        </div>
      ))}
      {contacts.map((c) => {
        const outcome = granted[c.id];
        const failure = grantError[c.id];
        if (outcome === undefined && (failure === undefined || failure === "")) return null;
        return (
          <div key={`${c.id}-outcome`} role="status" style={{ margin: "8px 0 0", padding: "10px 12px", borderRadius: 10, background: "var(--surface-2)", fontSize: 13.5, color: "var(--text)" }}>
            {failure !== undefined && failure !== "" ? (
              <span style={{ color: "var(--bad-fg)" }}>{failure}</span>
            ) : (
              <AccessOutcome name={c.fullName} result={outcome as ContactAccessResult} />
            )}
          </div>
        );
      })}

      <form onSubmit={(e) => void add(e)} style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 14 }}>
        <div style={{ flex: "2 1 150px", minWidth: 0 }}>
          <TextField
            label="Nombre del contacto"
            value={name}
            onChange={setName}
            uppercase
            required
            help="El nombre completo de a quién se le marca en una urgencia."
            error={form.errors.name}
            onClearError={() => form.clear("name")}
          />
        </div>
        <div style={{ flex: "1 1 120px", minWidth: 0 }}>
          <TextField
            label="Teléfono del contacto"
            value={phone}
            onChange={(v) => {
              setPhone(v);
              phoneShape.onChange("phone", v, {});
            }}
            onBlur={() => phoneShape.onBlur("phone", phone, {})}
            error={phoneShape.errors.phone}
            help="El número al que se marca, a diez dígitos."
          />
        </div>
        <div style={{ flex: "1 1 150px", minWidth: 0 }}>
          <EmailField
            label="Correo del contacto"
            value={email}
            onChange={(v) => {
              setEmail(v);
              emailShape.forgive(v);
            }}
            help="El correo de este contacto. Puede quedar vacío."
            error={emailShape.error}
          />
        </div>
        <div style={{ flex: "1 1 110px", minWidth: 0 }}>
          <SelectField
            label="Parentesco"
            value={relationship}
            onChange={(v) => setRelationship(v as Relationship)}
            options={RELATIONSHIPS.map((r) => ({ value: r.value, label: r.label }))}
          />
        </div>
        <EmergencyCheck label="Contacto de emergencia" checked={emergency} onChange={setEmergency} />
        <Btn type="submit" icon="plus" style={{ opacity: saving ? 0.5 : 1 }}>Agregar</Btn>
      </form>
      {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: "10px 0 0" }}>{error}</p>}
      {removal.latest !== undefined && (
        <UndoToast
          message={`Se quitó a ${removal.latest.fullName}`}
          onUndo={() => {
            const back = removal.undo() ?? [];
            setContacts((prev) => [...prev, ...back]);
          }}
          onDismiss={removal.dismiss}
        />
      )}
    </div>
  );
}

function displayName(c: Contact): string {
  return c.fullName !== "" ? c.fullName : "Cuenta vinculada";
}

const ROW_ACTION = { height: 36, padding: "0 12px", fontSize: 13.5 } as const;

function EmergencyCheck({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }): ReactElement {
  return (
    <label style={{ display: "inline-flex", alignItems: "center", gap: 8, minHeight: 44, fontSize: 14, color: "var(--text)", cursor: "pointer" }}>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} aria-label={label} style={{ width: 20, height: 20, accentColor: "var(--forest)" }} />
      De emergencia
    </label>
  );
}

/**
 * One contact, corrected where it sits: four fields and a mark do not earn a
 * dialog. Nothing is sent until "Guardar contacto".
 */
function ContactEditRow({
  contact,
  onSave,
  onCancel,
}: {
  contact: Contact;
  onSave: (edit: ContactEdit) => Promise<void>;
  onCancel: () => void;
}): ReactElement {
  const who = displayName(contact);
  const [name, setName] = useState(contact.fullName);
  const [phone, setPhone] = useState(contact.phone);
  const [email, setEmail] = useState(contact.email);
  const [relationship, setRelationship] = useState<Relationship>(contact.relationship);
  const [emergency, setEmergency] = useState(contact.isEmergency === true);
  const form = useRequired(contact.hasAccount ? NOTHING_REQUIRED : CONTACT_REQUIRED);
  const emailShape = useEmailShapeOnSave();
  const phoneShape = useFormatError(PHONE_RULES);
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState<string | undefined>(undefined);

  const save = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    const complete = form.check({ name });
    const shaped = emailShape.check(email);
    const phoned = phoneShape.checkAll({ phone });
    if (!complete || !shaped || !phoned || saving) return;
    setSaving(true);
    setFailure(undefined);
    try {
      await onSave({
        relationship,
        fullName: name.trim(),
        phone: phone.trim() === "" ? "" : phoneDigits(phone),
        email: email.trim(),
        isEmergency: emergency,
      });
    } catch (cause) {
      setFailure((cause instanceof Error ? spanishServerReason(cause.message) : undefined) ?? "No se pudo guardar el contacto.");
      setSaving(false);
    }
  };

  return (
    <form
      onSubmit={(e) => void save(e)}
      onKeyDown={(e) => {
        if (e.key === "Escape") onCancel();
      }}
      style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end", padding: "11px 0", borderBottom: "1px solid var(--border)" }}
    >
      <div style={{ flex: "2 1 150px", minWidth: 0 }}>
        <TextField label={`Nombre de ${who}`} value={name} onChange={setName} uppercase required={!contact.hasAccount} error={form.errors.name} onClearError={() => form.clear("name")} />
      </div>
      <div style={{ flex: "1 1 120px", minWidth: 0 }}>
        <TextField
          label={`Teléfono de ${who}`}
          value={phone}
          onChange={(v) => {
            setPhone(v);
            phoneShape.onChange("phone", v, {});
          }}
          onBlur={() => phoneShape.onBlur("phone", phone, {})}
          error={phoneShape.errors.phone}
        />
      </div>
      <div style={{ flex: "1 1 150px", minWidth: 0 }}>
        <EmailField
          label={`Correo de ${who}`}
          value={email}
          onChange={(v) => {
            setEmail(v);
            emailShape.forgive(v);
          }}
          error={emailShape.error}
        />
      </div>
      <div style={{ flex: "1 1 110px", minWidth: 0 }}>
        <SelectField
          label={`Parentesco de ${who}`}
          value={relationship}
          onChange={(v) => setRelationship(v as Relationship)}
          options={RELATIONSHIPS.map((r) => ({ value: r.value, label: r.label }))}
        />
      </div>
      <EmergencyCheck label={`Contacto de emergencia de ${who}`} checked={emergency} onChange={setEmergency} />
      <Btn type="submit" icon="check" style={{ opacity: saving ? 0.5 : 1 }}>Guardar contacto</Btn>
      <Btn kind="ghost" onClick={onCancel}>Cancelar</Btn>
      {failure !== undefined && <p role="alert" style={{ flexBasis: "100%", color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{failure}</p>}
    </form>
  );
}

/**
 * What to tell the office after it pressed the button.
 *
 * Every branch here exists because the wrong words cost somebody a week. "Correo
 * enviado" about a message that never left is how a family waits for a link that
 * was never sent; the same phrase about Google's message is how they wait for one
 * sitting in their spam folder. So each channel says what it actually did, and
 * the only case that can promise delivery is the school's own mail server.
 */
function AccessOutcome({ name, result }: { name: string; result: ContactAccessResult }): ReactElement {
  const { contact, invitation } = result;
  const who = name !== "" ? name : "El contacto";

  // Nothing was sent, and nothing needed to be: a parent with two children at
  // the school signs in once. Saying "invitación enviada" would have the office
  // waiting for a message nobody wrote.
  if (invitation === null) {
    return (
      <span>
        <strong>{who}</strong> ya tenía cuenta con ese correo. Quedó vinculado — entra con la
        contraseña que ya usa.
      </span>
    );
  }

  if (!invitation.sent) {
    return (
      <div style={{ display: "grid", gap: 8 }}>
        <span style={{ color: "var(--warn-fg)" }}>
          <strong>No se pudo enviar</strong> el correo{invitation.sendError !== undefined && invitation.sendError !== "" ? ` (${invitation.sendError})` : ""}.
          La cuenta sí quedó creada: copia esta liga y entrégasela a {who}.
        </span>
        <InviteLink url={invitation.url} />
      </div>
    );
  }

  // The provider's channel: Google mails its own link from a shared sender that
  // Gmail files as spam, and answers 200 even for an address with no account —
  // so "sent" here means accepted, never delivered. The office has to know.
  if (invitation.via === "provider") {
    return (
      <span>
        Le pedimos a Google que le mande la liga a <strong>{contact.email}</strong>. Suele caer
        en <strong>spam</strong> — pídele a {who} que la busque ahí, y que la abra dentro de una hora.
      </span>
    );
  }

  return (
    <span>
      Le enviamos la liga a <strong>{contact.email}</strong>. {who} la abre y elige su contraseña.
    </span>
  );
}

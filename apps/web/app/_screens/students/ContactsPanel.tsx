"use client";

import { useCallback, useEffect, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { EmailField, SelectField, TextField, emailShapeError, useFocusFirstInvalid, useRequired } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { addContact, inviteContactToApp, listContacts, type Contact, type ContactAccessResult, type Relationship } from "../../students/api";
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

export function ContactsPanel({ studentId }: ContactsPanelProps): ReactElement {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [email, setEmail] = useState("");
  const [relationship, setRelationship] = useState<Relationship>("other");
  const [error, setError] = useState<string | undefined>(undefined);
  const form = useRequired(CONTACT_REQUIRED);
  // El correo puede quedar vacío, pero si se escribe tiene que tener forma de
  // correo: se revisa aquí, al guardar, porque Enter guarda sin salir del campo.
  const [emailError, setEmailError] = useState<string | undefined>(undefined);
  const focusFirstInvalid = useFocusFirstInvalid();
  const [saving, setSaving] = useState(false);
  // Which contact is being given access, and what came of it. Keyed by contact
  // so a second press elsewhere never shows the first one's outcome.
  const [granting, setGranting] = useState<string | undefined>(undefined);
  const [granted, setGranted] = useState<Record<string, ContactAccessResult>>({});
  const [grantError, setGrantError] = useState<Record<string, string>>({});

  const reload = useCallback(() => {
    listContacts(API_URL, studentId)
      .then(setContacts)
      .catch(() => setError("No pudimos leer los contactos."));
  }, [studentId]);

  useEffect(reload, [reload]);

  const add = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    // Se marca EL CAMPO, no una frase al pie. El asterisco promete que el rojo
    // va a aparecer donde falta, y un mensaje abajo obliga a buscar cuál de los
    // cuatro campos era.
    const badEmail = emailShapeError(email);
    const complete = form.check({ name });
    setEmailError(badEmail);
    if (!complete || badEmail !== undefined) {
      focusFirstInvalid();
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

  const giveAccess = async (contact: Contact): Promise<void> => {
    setGranting(contact.id);
    setGrantError((prev) => ({ ...prev, [contact.id]: "" }));
    try {
      const result = await inviteContactToApp(API_URL, studentId, contact.id);
      setGranted((prev) => ({ ...prev, [contact.id]: result }));
      reload();
    } catch (cause) {
      setGrantError((prev) => ({
        ...prev,
        [contact.id]: cause instanceof Error ? cause.message : "No se pudo dar el acceso.",
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
          <TextField label="Teléfono del contacto" value={phone} onChange={setPhone} help="El número al que se marca, a diez dígitos." />
        </div>
        <div style={{ flex: "1 1 150px", minWidth: 0 }}>
          <EmailField
            label="Correo del contacto"
            value={email}
            onChange={setEmail}
            help="El correo de este contacto. Puede quedar vacío."
            error={emailError}
            onClearError={() => setEmailError(undefined)}
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
        <Btn type="submit" icon="plus" style={{ opacity: saving ? 0.5 : 1 }}>Agregar</Btn>
      </form>
      {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: "10px 0 0" }}>{error}</p>}
    </div>
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
        <input
          readOnly
          value={invitation.url}
          aria-label="Liga de acceso"
          onFocus={(e) => e.currentTarget.select()}
          style={{ width: "100%", padding: "8px 10px", borderRadius: 8, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 12.5, fontFamily: "monospace" }}
        />
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

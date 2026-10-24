"use client";

import { useRef, useState, type ReactElement, type ReactNode } from "react";

import { Btn } from "../../_design/Controls";
import { InviteLink } from "../../_design/InviteLink";
import { API_URL } from "../../api-url";
import { familyRefusal } from "../../families/api";
import { inviteContactToApp, type ContactAccessResult } from "../../students/api";
import { STUDENT_DRAFT } from "../../students/importIssues";

/**
 * "Dar acceso a la app": the one road to a family account, shared by a pupil's
 * contacts (ContactsPanel) and a family's adults (ADR 0041). One copy of the
 * staff-address question (C2-003), of the refusals and of the outcome, because
 * each of their sentences was paid for by somebody waiting for a mail.
 */

/** A draft's family access, refused before it is asked (and by the server, 409 STUDENT_DRAFT). */
export const DRAFT_INVITE = "Falta la CURP del alumno para invitarle";

/** iam's refusal of a staff address not yet confirmed (C2-003). */
const STAFF_ADDRESS_REASON = "belongs to a staff account";

/**
 * Whether the refusal is iam asking about a staff address. It reads the
 * server's own words (`detail`), never `message`: the error puts the Spanish
 * sentence for the screen there, and a 409's is a generic one.
 */
function isStaffAddressRefusal(cause: unknown): boolean {
  if (typeof cause !== "object" || cause === null || !("detail" in cause)) return false;
  return typeof cause.detail === "string" && cause.detail.includes(STAFF_ADDRESS_REASON);
}

function isDraftRefusal(cause: unknown): boolean {
  return typeof cause === "object" && cause !== null && "code" in cause && cause.code === STUDENT_DRAFT;
}

/** The contact row the invite goes to. */
export interface InviteTarget {
  studentId: string;
  contactId: string;
}

export interface ContactInvite {
  /** The key whose invite is in flight. */
  granting: string | undefined;
  granted: Readonly<Record<string, ContactAccessResult>>;
  grantError: Readonly<Record<string, string>>;
  /** Keys whose address iam said is a staff account, waiting for the director. */
  staffAddress: Readonly<Record<string, boolean>>;
  give: (key: string, target: InviteTarget, confirmStaffAccount?: boolean) => Promise<void>;
  /** Drops the staff question for a key ("No, corregir el correo"). */
  forgetStaff: (key: string) => void;
}

/**
 * The invite's state, keyed by whatever the screen lists (a contact id, a
 * person) so a second press elsewhere never shows the first one's outcome.
 * `onSettled` runs after every answer — a success or a refusal — so the screen
 * re-reads what changed (a family's account count).
 */
export function useContactInvite(onSettled: () => void): ContactInvite {
  const [granting, setGranting] = useState<string | undefined>(undefined);
  // The guard reads a ref: a second press in the same tick sees the first one's
  // state before React re-renders, which a closure over `granting` would not.
  const inFlight = useRef(false);
  const [granted, setGranted] = useState<Record<string, ContactAccessResult>>({});
  const [grantError, setGrantError] = useState<Record<string, string>>({});
  const [staffAddress, setStaffAddress] = useState<Record<string, boolean>>({});

  const give = async (key: string, target: InviteTarget, confirmStaffAccount = false): Promise<void> => {
    if (inFlight.current) return;
    inFlight.current = true;
    setGranting(key);
    setGrantError((prev) => ({ ...prev, [key]: "" }));
    setStaffAddress((prev) => ({ ...prev, [key]: false }));
    try {
      const result = await inviteContactToApp(API_URL, target.studentId, target.contactId, confirmStaffAccount ? { confirmStaffAccount: true } : {});
      setGranted((prev) => ({ ...prev, [key]: result }));
      onSettled();
    } catch (cause) {
      if (isStaffAddressRefusal(cause)) {
        setStaffAddress((prev) => ({ ...prev, [key]: true }));
        return;
      }
      setGrantError((prev) => ({ ...prev, [key]: isDraftRefusal(cause) ? DRAFT_INVITE : familyRefusal(cause, "No se pudo dar el acceso.") }));
      onSettled();
    } finally {
      inFlight.current = false;
      setGranting(undefined);
    }
  };

  const forgetStaff = (key: string): void => setStaffAddress((prev) => ({ ...prev, [key]: false }));
  return { granting, granted, grantError, staffAddress, give, forgetStaff };
}

/**
 * Security gate M2: beside "Dar acceso a la app", which children the account
 * will see ("Verá a ANA, LUIS y EVA"). The screens compute the line from the
 * family they already read (`accessReachLine`); nothing when it reaches one.
 */
export function AccessReach({ line }: { line: string | undefined }): ReactElement | null {
  if (line === undefined) return null;
  return <span style={{ fontSize: 13, color: "var(--text-muted)" }}>{line}</span>;
}

const NOTICE = { margin: "8px 0 0", padding: "10px 12px", borderRadius: 10, background: "var(--surface-2)", fontSize: 13.5, color: "var(--text)" } as const;

/** C2-003: the address is a staff account — same person, or a wrong address? */
export function StaffAddressPrompt({ email, onConfirm, onCorrect, correctLabel = "No, corregir el correo" }: {
  email: string;
  onConfirm: () => void;
  onCorrect: () => void;
  correctLabel?: string;
}): ReactElement {
  return (
    <div role="status" style={{ ...NOTICE, display: "grid", gap: 8 }}>
      <span>
        <strong>{email}</strong> es la cuenta de alguien del personal. Si es la misma persona —por ejemplo,
        un maestro que también es papá o mamá—, usará esa misma cuenta para entrar a la app. Si no, corrige el correo del contacto.
      </span>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <Btn onClick={onConfirm}>Sí, es la misma persona</Btn>
        <Btn kind="ghost" onClick={onCorrect}>{correctLabel}</Btn>
      </div>
    </div>
  );
}

/** The invite's answer for one key: its refusal, or what was (not) sent. */
export function InviteResult({ name, outcome, failure }: { name: string; outcome?: ContactAccessResult; failure?: string }): ReactElement | null {
  if (outcome === undefined && (failure === undefined || failure === "")) return null;
  let body: ReactNode;
  if (failure !== undefined && failure !== "") body = <span style={{ color: "var(--bad-fg)" }}>{failure}</span>;
  else body = <AccessOutcome name={name} result={outcome as ContactAccessResult} />;
  return <div role="status" style={NOTICE}>{body}</div>;
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
  // the school signs in once.
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
  // Gmail files as spam, so "sent" here means accepted, never delivered.
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

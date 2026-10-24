"use client";

import { useId, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import type { FamilyContactRef, FamilyPerson } from "../../families/api";
import { relationshipLabel } from "../../students/relationship";
import { AccessReach, InviteResult, StaffAddressPrompt, type ContactInvite } from "../students/contactInvite";

const ACTION = { height: 44, padding: "0 14px", fontSize: 14, gap: 7 } as const;
const HINT = { fontSize: 13, color: "var(--text-faint)" } as const;

/**
 * A person's React key: their smallest contact id. Contact rows outlive what
 * a re-read changes about the person (an account just given, a new address),
 * so the row is the same row before and after an invite.
 */
export function personKey(person: FamilyPerson): string {
  return [...person.contacts.map((c) => c.contactId)].sort()[0] ?? person.fullName;
}

/**
 * The contact the invite state of this person lives under: the one actually
 * invited when there is one (found across all their rows), else their first.
 * Keyed by contact, never by account or address: both change when the invite
 * succeeds, and the outcome (the link to copy) must survive that re-read.
 */
function inviteKey(person: FamilyPerson, invite: ContactInvite): string | undefined {
  const known = person.contacts.find((c) =>
    invite.granted[c.contactId] !== undefined || (invite.grantError[c.contactId] ?? "") !== "" || invite.staffAddress[c.contactId] === true || invite.granting === c.contactId);
  return (known ?? person.contacts[0])?.contactId;
}

export interface AdultRowProps {
  person: FamilyPerson;
  invite: ContactInvite;
  /** The family already holds two accounts: nobody else is offered one. */
  full: boolean;
  /** Absent without guardians:write. */
  onAskRevoke?: () => void;
  canGrant: boolean;
  /** Security gate M2: which children an account given here will see; absent for one. */
  accessReach?: string;
}

/** One adult: how to reach them, whether they have the app, and the one action that applies. */
export function AdultRow({ person, invite, full, onAskRevoke, canGrant, accessReach }: AdultRowProps): ReactElement {
  const nameId = useId();
  const first = person.contacts[0];
  const key = inviteKey(person, invite) ?? "";
  const reach = [relationshipLabel(person.relationship), person.phone, person.email].filter((s) => s !== "").join(" · ");

  let action: ReactElement | null = null;
  if (person.hasApp && onAskRevoke !== undefined && person.guardianUserId !== undefined) {
    action = <Btn kind="ghost" onClick={onAskRevoke} style={ACTION}>Quitar acceso</Btn>;
  } else if (!person.hasApp && canGrant) {
    if (full) action = <span style={HINT}>Ya tiene 2 cuentas de la app</span>;
    else if (person.email === "" || first === undefined) action = <span style={HINT}>Falta el correo para invitarle</span>;
    else action = (
      <>
        <Btn kind="ghost" icon="mail" busy={invite.granting === key} onClick={() => void invite.give(first.contactId, first)} style={ACTION}>
          {invite.granting === key ? "Enviando…" : "Dar acceso a la app"}
        </Btn>
        <AccessReach line={accessReach} />
      </>
    );
  }

  return (
    <li aria-labelledby={nameId} style={{ display: "grid", gap: 4, padding: "10px 0", borderBottom: "1px solid var(--border)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div id={nameId} style={{ fontSize: 15, fontWeight: 600, color: "var(--text)" }}>{person.fullName !== "" ? person.fullName : "Cuenta vinculada"}</div>
          <div style={{ fontSize: 13, color: "var(--text-muted)" }}>{reach}</div>
        </div>
        {person.hasApp && (
          <span title="Puede entrar a la app" style={{ display: "inline-flex", alignItems: "center", gap: 5, padding: "4px 10px", borderRadius: 99, background: "var(--ok-bg)", color: "var(--ok-fg)", fontSize: 12, fontWeight: 700 }}>
            <Icon name="check" size={13} stroke={2.6} />App
          </span>
        )}
        {action}
      </div>
      {invite.staffAddress[key] === true && (
        <StaffAddressPrompt
          email={person.email}
          onConfirm={() => void invite.give(key, person.contacts.find((c) => c.contactId === key) ?? (first as FamilyContactRef), true)}
          onCorrect={() => invite.forgetStaff(key)}
          correctLabel="No, es otra persona"
        />
      )}
      <InviteResult name={person.fullName} outcome={invite.granted[key]} failure={invite.grantError[key]} />
    </li>
  );
}

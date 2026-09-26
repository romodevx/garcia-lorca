"use client";

import type { ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { InviteLink } from "../../_design/InviteLink";
import type { Teacher } from "../../teachers/api";
import type { TeacherAccessState } from "./useTeacherAccess";

export interface TeacherRowProps {
  teacher: Teacher;
  access: TeacherAccessState;
  granting: boolean;
  error: string | undefined;
  inviteUrl: string | undefined;
  /** Why the mail failed; undefined when no mail is configured at all. */
  sendError?: string;
  /** The server's question when an existing account would be linked. */
  confirmation?: string;
  onConfirmReuse?: () => void;
  onCancelReuse?: () => void;
  /** Absent when this principal may not give access (teachers:write + users:write). */
  onGrantAccess?: () => void;
  onOpenLoad?: () => void;
}

const ROW_BUTTON = { minHeight: 44, padding: "0 14px", borderRadius: "var(--r-pill)", border: "1.5px solid var(--forest)", color: "var(--forest)", fontWeight: 600, fontSize: 13.5, cursor: "pointer" } as const;

/** One roster row: who, whether they can sign in, and what can be done about it. */
export function TeacherRow({ teacher, access, granting, error, inviteUrl, sendError, confirmation, onConfirmReuse, onCancelReuse, onGrantAccess, onOpenLoad }: TeacherRowProps): ReactElement {
  const name = `${teacher.firstName} ${teacher.lastName}`;
  const action = ACTION[access];
  const offerAccess = onGrantAccess !== undefined && action !== undefined && confirmation === undefined;

  return (
    <li style={{ display: "grid", gap: 8, padding: "6px 6px 6px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", fontSize: 14 }}>
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
        <span style={{ minWidth: 0, overflowWrap: "anywhere" }}>
          <span style={{ fontWeight: 600 }}>{name}</span>{" "}
          <span style={{ color: "var(--text-muted)" }}>· {teacher.email}</span>{" "}
          <AccessPill access={access} />
        </span>
        <span style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          {offerAccess && (
            <button type="button" onClick={onGrantAccess} aria-busy={granting} aria-label={`${action} a ${name}`} style={{ ...ROW_BUTTON, opacity: granting ? 0.6 : 1 }}>
              {granting ? "Enviando…" : action}
            </button>
          )}
          {onOpenLoad !== undefined && (
            <button type="button" onClick={onOpenLoad} aria-label={`Carga académica de ${name}`} style={ROW_BUTTON}>
              Carga académica
            </button>
          )}
        </span>
      </div>
      {confirmation !== undefined && (
        <div role="group" aria-label={`Confirmar acceso de ${name}`} style={{ display: "grid", gap: 8, padding: "10px 12px", borderRadius: "var(--r-md)", background: "var(--surface)", border: "1.5px solid var(--border)" }}>
          <p style={{ margin: 0, fontSize: 13.5, color: "var(--text)" }}>{confirmation}</p>
          <span style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
            <button type="button" onClick={onConfirmReuse} aria-busy={granting} style={{ ...ROW_BUTTON, background: "var(--forest)", color: "#fff", opacity: granting ? 0.6 : 1 }}>
              {granting ? "Enviando…" : "Vincular y dar acceso"}
            </button>
            <button type="button" onClick={onCancelReuse} style={{ minHeight: 44, padding: "0 10px", color: "var(--text-muted)", fontWeight: 600, fontSize: 13.5, cursor: "pointer" }}>
              Cancelar
            </button>
          </span>
        </div>
      )}
      {error !== undefined && (
        <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 13.5 }}>{error}</p>
      )}
      {inviteUrl !== undefined && (
        <div style={{ display: "grid", gap: 6 }}>
          <span style={{ fontSize: 13, color: sendError === undefined ? "var(--text-muted)" : "var(--warn-fg)" }}>
            {sendError === undefined
              ? `No hay correo configurado: copia esta liga y entrégasela a ${teacher.firstName}.`
              : `No pudimos enviar el correo: ${sendError}. Copia esta liga y entrégasela.`}
          </span>
          <InviteLink url={inviteUrl} />
        </div>
      )}
    </li>
  );
}

/**
 * What the row offers per state: a login to create, or an invitation to send
 * again — also for a login this page cannot read, since the server answers
 * with its real state. An active login needs nothing, a suspension is lifted
 * in Usuarios, and a status iam named but this build does not know is never
 * guessed at.
 */
const ACTION: Record<TeacherAccessState, string | undefined> = {
  none: "Dar acceso al sistema",
  invited: "Reenviar invitación",
  invitationSent: "Reenviar invitación",
  linked: "Reenviar invitación",
  unrecognised: undefined,
  active: undefined,
  suspended: undefined,
};

const PILL: Record<TeacherAccessState, { label: string; bg: string; fg: string; check: boolean }> = {
  none: { label: "Sin acceso", bg: "var(--surface-3)", fg: "var(--text-muted)", check: false },
  linked: { label: "Vinculado", bg: "var(--surface-3)", fg: "var(--text)", check: false },
  unrecognised: { label: "Vinculado", bg: "var(--surface-3)", fg: "var(--text)", check: false },
  invited: { label: "Invitación pendiente", bg: "var(--warn-bg)", fg: "var(--warn-fg)", check: false },
  invitationSent: { label: "Invitación enviada", bg: "var(--warn-bg)", fg: "var(--warn-fg)", check: false },
  active: { label: "Con acceso", bg: "var(--ok-bg)", fg: "var(--ok-fg)", check: true },
  suspended: { label: "Acceso suspendido", bg: "var(--bad-bg)", fg: "var(--bad-fg)", check: false },
};

function AccessPill({ access }: { access: TeacherAccessState }): ReactElement {
  const pill = PILL[access];
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 5, padding: "2px 9px", borderRadius: 99, fontSize: 12, fontWeight: 700, whiteSpace: "nowrap", background: pill.bg, color: pill.fg }}>
      {pill.check && <Icon name="check" size={12} stroke={2.6} />}
      {pill.label}
    </span>
  );
}

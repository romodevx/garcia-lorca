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
  /** Absent when this principal may not give access (teachers:write + users:write). */
  onGrantAccess?: () => void;
  onOpenLoad?: () => void;
}

const ROW_BUTTON = { minHeight: 44, padding: "0 14px", borderRadius: "var(--r-pill)", border: "1.5px solid var(--forest)", color: "var(--forest)", fontWeight: 600, fontSize: 13.5, cursor: "pointer" } as const;

/** One roster row: who, whether they can sign in, and what can be done about it. */
export function TeacherRow({ teacher, access, granting, error, inviteUrl, onGrantAccess, onOpenLoad }: TeacherRowProps): ReactElement {
  const name = `${teacher.firstName} ${teacher.lastName}`;
  // An active login needs nothing; any other linked login may need its invitation again.
  const action = access === "none" ? "Dar acceso al sistema" : "Reenviar invitación";
  // Suspension is lifted in Usuarios, not by resending a link.
  const offerAccess = onGrantAccess !== undefined && access !== "active" && access !== "suspended";

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
      {error !== undefined && (
        <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 13.5 }}>{error}</p>
      )}
      {inviteUrl !== undefined && (
        <div style={{ display: "grid", gap: 6 }}>
          <span style={{ fontSize: 13, color: "var(--text-muted)" }}>No hay correo configurado: copia esta liga y entrégasela a {teacher.firstName}.</span>
          <InviteLink url={inviteUrl} />
        </div>
      )}
    </li>
  );
}

const PILL: Record<TeacherAccessState, { label: string; bg: string; fg: string; check: boolean }> = {
  none: { label: "Sin acceso", bg: "var(--surface-3)", fg: "var(--text-muted)", check: false },
  linked: { label: "Con acceso", bg: "var(--ok-bg)", fg: "var(--ok-fg)", check: true },
  invited: { label: "Con acceso", bg: "var(--ok-bg)", fg: "var(--ok-fg)", check: true },
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

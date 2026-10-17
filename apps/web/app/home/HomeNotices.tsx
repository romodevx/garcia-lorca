"use client";

import type { ReactElement } from "react";

import { Icon } from "../_design/Icon";

/*
 * The home's two notices, shared by the classic and the refreshed home (B9,
 * B10): moved here unchanged from Dashboard.tsx, so both say the same thing.
 */

// Shown when the caller's permissions could not be loaded.
//
// NOT the same as "you have no modules", and saying that would be a lie: the
// system does not know what this person can do, which is a fault on our side.
// The previous behaviour said nothing at all — it kept showing the loading
// skeleton — and a session broken this way was indistinguishable from a slow
// network for as long as anybody cared to wait.
export function PermissionsUnavailable({ onRetry }: { onRetry?: () => void }): ReactElement {
  return (
    <div
      role="alert"
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 10,
        padding: "48px 24px",
        borderRadius: "var(--r-lg)",
        background: "var(--surface)",
        border: "1px dashed var(--bad-fg)",
        color: "var(--text-muted)",
        textAlign: "center"
      }}
    >
      <Icon name="shield" size={30} stroke={2} />
      <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 18, color: "var(--text)" }}>
        No pudimos cargar tus permisos
      </div>
      <div style={{ fontSize: 13.5 }}>
        Tu sesión está iniciada, pero el sistema no pudo comprobar qué puedes ver. No es que no tengas módulos.
      </div>
      {onRetry !== undefined && (
        <button
          onClick={onRetry}
          style={{
            marginTop: 6,
            height: 40,
            padding: "0 18px",
            borderRadius: "var(--r-pill)",
            fontSize: 14,
            fontWeight: 600,
            color: "var(--forest)",
            background: "var(--surface-3)",
            border: "1px solid var(--border)",
            cursor: "pointer"
          }}
        >
          Reintentar
        </button>
      )}
    </div>
  );
}

// The scopes a guardian holds over their own children: their account in the
// family app, and their own uniform orders (migrations/iam/0014).
const GUARDIAN_PERMISSION_PREFIXES = ["family:", "uniforms_own:"] as const;

/**
 * A guardian's permissions are all guardian scopes: everything they can do lives
 * in the family app, and no administrator will grant them a web module.
 */
function isFamilyOnly(permissions: readonly string[]): boolean {
  return (
    permissions.some((p) => p.startsWith("family:")) &&
    permissions.every((p) => GUARDIAN_PERMISSION_PREFIXES.some((prefix) => p.startsWith(prefix)))
  );
}

// Shown to a signed-in user whose permissions match no module at all.
export function NoModules({ permissions }: { permissions: readonly string[] }): ReactElement {
  const family = isFamilyOnly(permissions);
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 10,
        padding: "48px 24px",
        borderRadius: "var(--r-lg)",
        background: "var(--surface)",
        border: "1px dashed var(--border)",
        color: "var(--text-muted)",
        textAlign: "center"
      }}
    >
      <Icon name="shield" size={30} stroke={2} />
      <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 18, color: "var(--text)" }}>
        {family ? "Aquí no hay módulos para familias" : "No tienes módulos asignados"}
      </div>
      <div style={{ fontSize: 13.5 }}>
        {family
          ? "Tu acceso es desde la app de García Lorca para familias."
          : "Solicita acceso a tu administrador para ver tus módulos aquí."}
      </div>
    </div>
  );
}

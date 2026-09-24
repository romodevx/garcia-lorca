"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { AdminUser, CreateUserInput, Role } from "../../admin/api";
import { EmailField, TextField } from "../../_design/form";
import { CheckList, EmptyHint, ErrorAlert, Section, StatusPill } from "./parts";

export interface UsersPanelProps {
  users: readonly AdminUser[];
  roles: readonly Role[];
  onCreate: (input: CreateUserInput) => Promise<void>;
  onSetStatus: (userId: string, status: "active" | "suspended") => Promise<void>;
  onSetRoles: (userId: string, roleIds: string[]) => Promise<void>;
}

/**
 * Users admin: lists login accounts (email, display name, status, roles),
 * creates a user (email + display name + a temporary password + a role
 * multi-select), suspends/reactivates an account, and edits a user's roles
 * inline. Dumb panel — data + async handlers come from the Admin orchestrator;
 * the gateway (behind module_iam + users:write) is the security boundary. The
 * temp password is sent once at creation and never echoed back.
 */
export function UsersPanel({ users, roles, onCreate, onSetStatus, onSetRoles }: UsersPanelProps): ReactElement {
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [tempPassword, setTempPassword] = useState("");
  const [roleIds, setRoleIds] = useState<ReadonlySet<string>>(new Set());
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const [busyId, setBusyId] = useState<string | undefined>(undefined);
  const [editingId, setEditingId] = useState<string | undefined>(undefined);
  const [editRoles, setEditRoles] = useState<ReadonlySet<string>>(new Set());

  const roleOptions = roles.map((r) => ({ value: r.id, label: r.name }));
  const roleName = (id: string): string => roles.find((r) => r.id === id)?.name ?? id;

  const toggle = (set: ReadonlySet<string>, value: string): Set<string> => {
    const next = new Set(set);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    return next;
  };

  const canSubmit =
    email.trim() !== "" && displayName.trim() !== "" && tempPassword.trim() !== "" && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({
        email: email.trim(),
        displayName: displayName.trim(),
        tempPassword: tempPassword.trim(),
        roleIds: [...roleIds],
      });
      setEmail("");
      setDisplayName("");
      setTempPassword("");
      setRoleIds(new Set());
    } catch {
      setError("No se pudo crear el usuario. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  const changeStatus = async (u: AdminUser): Promise<void> => {
    const next = u.status === "active" ? "suspended" : "active";
    setBusyId(u.id);
    try {
      await onSetStatus(u.id, next);
    } finally {
      setBusyId(undefined);
    }
  };

  const startEditRoles = (u: AdminUser): void => {
    setEditingId(u.id);
    setEditRoles(new Set(u.roleIds ?? []));
  };

  const saveRoles = async (userId: string): Promise<void> => {
    setBusyId(userId);
    try {
      await onSetRoles(userId, [...editRoles]);
      setEditingId(undefined);
    } finally {
      setBusyId(undefined);
    }
  };

  return (
    <Section title="Usuarios" subtitle="Cuentas de acceso al sistema.">
      {users.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 8 }}>
          {users.map((u) => (
            <li key={u.id} style={{ padding: "10px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 10, flexWrap: "wrap" }}>
                <div style={{ fontSize: 14 }}>
                  <span style={{ fontWeight: 600 }}>{u.displayName}</span>{" "}
                  <span style={{ color: "var(--text-muted)" }}>· {u.email}</span>
                  <div style={{ marginTop: 4, display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                    <StatusPill status={u.status} />
                    <span style={{ fontSize: 12.5, color: "var(--text-muted)" }}>
                      {(u.roleIds ?? []).length > 0 ? (u.roleIds ?? []).map(roleName).join(", ") : "Sin roles"}
                    </span>
                  </div>
                </div>
                <div style={{ display: "flex", gap: 8 }}>
                  <button
                    onClick={() => (editingId === u.id ? setEditingId(undefined) : startEditRoles(u))}
                    aria-label={`Editar roles de ${u.displayName}`}
                    disabled={busyId === u.id}
                    style={{ fontSize: 13, fontWeight: 600, color: "var(--forest)", opacity: busyId === u.id ? 0.5 : 1 }}
                  >
                    {editingId === u.id ? "Cancelar" : "Editar roles"}
                  </button>
                  <button
                    onClick={() => void changeStatus(u)}
                    aria-label={`${u.status === "active" ? "Suspender" : "Reactivar"} a ${u.displayName}`}
                    disabled={busyId === u.id}
                    style={{ fontSize: 13, fontWeight: 600, color: u.status === "active" ? "var(--bad-fg)" : "var(--forest)", opacity: busyId === u.id ? 0.5 : 1 }}
                  >
                    {u.status === "active" ? "Suspender" : "Reactivar"}
                  </button>
                </div>
              </div>
              {editingId === u.id && (
                <div style={{ marginTop: 12 }}>
                  <CheckList
                    legend={`Roles de ${u.displayName}`}
                    options={roleOptions}
                    selected={editRoles}
                    onToggle={(v) => setEditRoles((prev) => toggle(prev, v))}
                    disabled={busyId === u.id}
                  />
                  <div style={{ marginTop: 10 }}>
                    <Btn type="button" icon="check" disabled={busyId === u.id} onClick={() => void saveRoles(u.id)}>
                      Guardar roles
                    </Btn>
                  </div>
                </div>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay usuarios. Crea el primero abajo.</EmptyHint>
      )}

      <form aria-label="Crear usuario" onSubmit={submit}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, marginBottom: 12 }}>
          <EmailField label="Correo" value={email} onChange={setEmail} help="El correo con el que esta persona entrará al sistema." />
          <TextField label="Nombre para mostrar" value={displayName} onChange={setDisplayName} help="El nombre que verán los demás en el sistema." />
        </div>
        <div style={{ marginBottom: 12 }}>
          <TextField
            label="Contraseña temporal"
            value={tempPassword}
            onChange={setTempPassword}
            help="Con esto entra la primera vez. El sistema le pedirá cambiarla en ese primer ingreso. Mínimo 8 caracteres."
          />
        </div>
        {roleOptions.length > 0 && (
          <div style={{ marginBottom: 12 }}>
            <CheckList
              legend="Roles"
              options={roleOptions}
              selected={roleIds}
              onToggle={(v) => setRoleIds((prev) => toggle(prev, v))}
              disabled={pending}
            />
          </div>
        )}
        <Btn type="submit" icon="plus" disabled={!canSubmit}>
          Crear usuario
        </Btn>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

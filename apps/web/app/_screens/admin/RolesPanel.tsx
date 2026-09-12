"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateRoleInput, Role } from "../../admin/api";
import { PERMISSION_CATALOG } from "./catalog";
import { TextField } from "../../_design/form";
import { CheckList, EmptyHint, ErrorAlert, Section } from "./parts";

export interface RolesPanelProps {
  roles: readonly Role[];
  onCreate: (input: CreateRoleInput) => Promise<void>;
  onSetPermissions: (roleId: string, permissions: string[]) => Promise<void>;
}

// Catalog grouped for the checklist (resource sub-headings + labelled options).
const PERMISSION_GROUPS = PERMISSION_CATALOG.map((g) => ({
  heading: g.resource,
  options: g.permissions.map((p) => ({ value: p.value, label: p.label })),
}));

/**
 * Roles admin: lists roles, creates a role (name + a permission checklist drawn
 * from the Phase-6 catalog), and edits an existing role's permissions inline.
 * Dumb panel — data + async handlers come from the Admin orchestrator; the
 * gateway (behind module_iam + roles:write) is the security boundary.
 */
export function RolesPanel({ roles, onCreate, onSetPermissions }: RolesPanelProps): ReactElement {
  const [name, setName] = useState("");
  const [perms, setPerms] = useState<ReadonlySet<string>>(new Set());
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  // Inline permission editor for an existing role.
  const [editingId, setEditingId] = useState<string | undefined>(undefined);
  const [editPerms, setEditPerms] = useState<ReadonlySet<string>>(new Set());
  const [editPending, setEditPending] = useState(false);
  const [editError, setEditError] = useState<string | undefined>(undefined);

  const toggle = (set: ReadonlySet<string>, value: string): Set<string> => {
    const next = new Set(set);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    return next;
  };

  const canSubmit = name.trim() !== "" && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ name: name.trim(), permissions: [...perms] });
      setName("");
      setPerms(new Set());
    } catch {
      setError("No se pudo crear el rol. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  const startEdit = (role: Role): void => {
    setEditingId(role.id);
    setEditPerms(new Set(role.permissions ?? []));
    setEditError(undefined);
  };

  const saveEdit = async (roleId: string): Promise<void> => {
    setEditPending(true);
    setEditError(undefined);
    try {
      await onSetPermissions(roleId, [...editPerms]);
      setEditingId(undefined);
    } catch {
      setEditError("No se pudieron guardar los permisos.");
    } finally {
      setEditPending(false);
    }
  };

  return (
    <Section title="Roles" subtitle="Conjuntos de permisos que asignas a los usuarios.">
      {roles.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 8 }}>
          {roles.map((r) => (
            <li key={r.id} style={{ padding: "10px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 10 }}>
                <span style={{ fontSize: 14 }}>
                  <span style={{ fontWeight: 600 }}>{r.name}</span>{" "}
                  <span style={{ color: "var(--text-muted)" }}>· {(r.permissions ?? []).length} permiso(s)</span>
                </span>
                <button
                  onClick={() => (editingId === r.id ? setEditingId(undefined) : startEdit(r))}
                  aria-label={`Editar permisos de ${r.name}`}
                  style={{ fontSize: 13, fontWeight: 600, color: "var(--forest)" }}
                >
                  {editingId === r.id ? "Cancelar" : "Editar permisos"}
                </button>
              </div>
              {editingId === r.id && (
                <div style={{ marginTop: 12 }}>
                  <CheckList
                    legend={`Permisos de ${r.name}`}
                    groups={PERMISSION_GROUPS}
                    selected={editPerms}
                    onToggle={(v) => setEditPerms((prev) => toggle(prev, v))}
                    disabled={editPending}
                  />
                  <div style={{ marginTop: 10 }}>
                    <Btn type="button" icon="check" disabled={editPending} onClick={() => void saveEdit(r.id)}>
                      Guardar permisos
                    </Btn>
                  </div>
                  {editError !== undefined && <ErrorAlert message={editError} />}
                </div>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay roles. Crea el primero abajo.</EmptyHint>
      )}

      <form aria-label="Crear rol" onSubmit={submit}>
        <div style={{ marginBottom: 12 }}>
          <TextField label="Nombre del rol" value={name} onChange={setName} placeholder="Coordinación académica" />
        </div>
        <div style={{ marginBottom: 12 }}>
          <CheckList
            legend="Permisos"
            groups={PERMISSION_GROUPS}
            selected={perms}
            onToggle={(v) => setPerms((prev) => toggle(prev, v))}
            disabled={pending}
          />
        </div>
        <Btn type="submit" icon="plus" disabled={!canSubmit}>
          Crear rol
        </Btn>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

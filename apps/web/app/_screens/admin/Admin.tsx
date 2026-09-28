"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import {
  createRole,
  createUser,
  linkTeacherUser,
  listRoles,
  listUsers,
  setRolePermissions,
  setUserRoles,
  setUserStatus,
  type AdminUser,
  type CreateRoleInput,
  type CreateUserInput,
  type Role,
} from "../../admin/api";
import { listTeachers, type Teacher } from "../../teachers/api";
import { RolesPanel } from "./RolesPanel";
import { TeacherLinkPanel } from "./TeacherLinkPanel";
import { UsersPanel } from "./UsersPanel";

import { API_URL } from "../../api-url";

// Gateway base URL for the browser (public env; matches AppShell/compose).

export interface AdminProps {
  onBack: () => void;
}

/**
 * Admin surface (module_iam): users, roles, and the teacher↔user link. This is
 * the single fetch owner — it loads roles/users/teachers and passes plain data +
 * async handlers down to small presentational panels (RolesPanel, UsersPanel,
 * TeacherLinkPanel), mirroring the config Catalog orchestrator. The gateway
 * (behind module_iam + the users/roles permissions, and config:write for the
 * link) is the security boundary; this screen is UX only.
 */
export function Admin({ onBack }: AdminProps): ReactElement {
  const [roles, setRoles] = useState<Role[]>([]);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);

  useEffect(() => {
    let active = true;
    setLoading(true);
    Promise.all([listRoles(API_URL), listUsers(API_URL), listTeachers(API_URL)])
      .then(([rs, us, ts]) => {
        if (!active) return;
        setRoles(rs);
        setUsers(us);
        setTeachers(ts);
      })
      .catch(() => active && setLoadError("No se pudo cargar la administración."))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  const handleCreateRole = useCallback(async (input: CreateRoleInput): Promise<void> => {
    const created = await createRole(API_URL, input);
    setRoles((prev) => [...prev, created]);
  }, []);

  const handleSetPermissions = useCallback(async (roleId: string, permissions: string[]): Promise<void> => {
    const updated = await setRolePermissions(API_URL, roleId, permissions);
    setRoles((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
  }, []);

  const handleCreateUser = useCallback(async (input: CreateUserInput): Promise<void> => {
    const created = await createUser(API_URL, input);
    setUsers((prev) => [...prev, created]);
  }, []);

  const handleSetStatus = useCallback(async (userId: string, status: "active" | "suspended"): Promise<void> => {
    const updated = await setUserStatus(API_URL, userId, status);
    setUsers((prev) => prev.map((u) => (u.id === updated.id ? updated : u)));
  }, []);

  const handleSetRoles = useCallback(async (userId: string, roleIds: string[]): Promise<void> => {
    const updated = await setUserRoles(API_URL, userId, roleIds);
    setUsers((prev) => prev.map((u) => (u.id === updated.id ? updated : u)));
  }, []);

  const handleLink = useCallback(async (teacherId: string, userId: string): Promise<void> => {
    const updated = await linkTeacherUser(API_URL, teacherId, userId);
    setTeachers((prev) => prev.map((t) => (t.id === updated.id ? updated : t)));
  }, []);

  return (
    <Page>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--surface-3)", color: "var(--text-muted)" }}>
          <Icon name="shield" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Administración</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Usuarios, roles y vínculos con profesores</p>
        </div>
      </div>

      {loading ? (
        <p className="fade-up" style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando administración…</p>
      ) : loadError !== undefined ? (
        <p role="alert" className="fade-up" style={{ color: "var(--bad-fg)", fontSize: 15 }}>{loadError}</p>
      ) : (
        <div className="fade-up" style={{ display: "grid", gap: 18 }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(min(340px, 100%), 1fr))", gap: 18 }}>
            <RolesPanel roles={roles} onCreate={handleCreateRole} onSetPermissions={handleSetPermissions} />
            <UsersPanel
              users={users}
              roles={roles}
              onCreate={handleCreateUser}
              onSetStatus={handleSetStatus}
              onSetRoles={handleSetRoles}
            />
          </div>
          <TeacherLinkPanel teachers={teachers} users={users} onLink={handleLink} />
        </div>
      )}
    </Page>
  );
}

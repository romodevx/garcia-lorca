"use client";

import { useCallback, useEffect, useMemo, useState, type CSSProperties, type ReactElement } from "react";
import { Avatar } from "../../_design/Brand";
import { Btn } from "../../_design/Controls";
import { Icon, type IconName } from "../../_design/Icon";
import type { ColorKey } from "../../_design/colors";
import {
  createRole,
  createUser,
  listRoles,
  listUsers,
  setRolePermissions,
  setUserStatus,
  setUserRoles,
  type AdminUser,
  type CreateRoleInput,
  type CreateUserInput,
  type Invitation,
  type UserStatus,
  type InviteUserInput,
  inviteUser,
  resendInvitation,
  type Role,
} from "../../admin/api";
import { providerLoginEnabled } from "../../auth/providerFlag";
import { PERMISSION_CATALOG } from "./catalog";

import { API_URL } from "../../api-url";

const AV: ColorKey[] = ["lime", "forest", "orange", "purple", "yellow"];

const qb: CSSProperties = { width: 36, height: 36, borderRadius: "50%", display: "grid", placeItems: "center", background: "var(--surface-3)", border: "1px solid var(--border)", color: "var(--text-muted)", cursor: "pointer" };
const fi: CSSProperties = { width: "100%", height: 50, padding: "0 16px", borderRadius: "var(--r-md)", border: "1.5px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 15.5, outline: "none" };
const chip = (on: boolean, color: string): CSSProperties => ({ height: 40, padding: "0 15px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer", border: `1.5px solid ${on ? `var(--${color})` : "var(--border)"}`, background: on ? `color-mix(in oklab, var(--${color}) 12%, var(--surface))` : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" });

/**
 * The pill was a boolean and the world has three states.
 *
 * "Invitado" is the one that did not exist: an account that was created and
 * that nobody has opened. Drawing it as "Suspendido" — which is what a boolean
 * had to do — told the director that somebody had been shut out when in fact
 * they had never been let in.
 */
const PILL = {
  active: { label: "Activo", bg: "var(--ok-bg)", fg: "var(--ok-fg)", dot: "var(--forest)" },
  invited: { label: "Invitado", bg: "var(--warn-bg)", fg: "var(--warn-fg)", dot: "var(--yellow)" },
  suspended: { label: "Suspendido", bg: "var(--bad-bg)", fg: "var(--bad-fg)", dot: "var(--red)" },
  unknown: { label: "Desconocido", bg: "var(--bad-bg)", fg: "var(--bad-fg)", dot: "var(--red)" },
} as const;

function StatusPill({ status }: { status: UserStatus }): ReactElement {
  const p = PILL[status] ?? PILL.unknown;
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 7, padding: "5px 12px 5px 10px", borderRadius: 99, background: p.bg, color: p.fg, fontWeight: 700, fontSize: 13, fontFamily: "var(--font-display)" }}>
      <span style={{ width: 9, height: 9, borderRadius: 99, background: p.dot, boxShadow: `0 0 0 3px color-mix(in oklab, ${p.dot} 22%, transparent)` }} />{p.label}
    </span>
  );
}

function Drawer({ title, subtitle, icon, onClose, children, footer }: { title: string; subtitle: string; icon: IconName; onClose: () => void; children: ReactElement; footer: ReactElement }): ReactElement {
  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, zIndex: 70, background: "rgba(6,20,12,.5)", backdropFilter: "blur(4px)", display: "flex", justifyContent: "flex-end" }}>
      <div onClick={(e) => e.stopPropagation()} role="dialog" aria-label={title} style={{ width: "min(520px, 100%)", height: "100%", background: "var(--surface)", borderLeft: "1px solid var(--border)", boxShadow: "var(--shadow-pop)", display: "flex", flexDirection: "column", animation: "slideIn .28s var(--ease) both" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 14, padding: "22px 26px", borderBottom: "1px solid var(--border)" }}>
          <div style={{ width: 48, height: 48, borderRadius: 14, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff" }}><Icon name={icon} size={26} /></div>
          <div style={{ flex: 1 }}>
            <h2 style={{ fontSize: 21, fontWeight: 700, color: "var(--text)" }}>{title}</h2>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", marginTop: 2 }}>{subtitle}</p>
          </div>
          <button onClick={onClose} aria-label="Cerrar" style={{ ...qb, width: 40, height: 40 }}><Icon name="close" size={19} /></button>
        </div>
        <div style={{ flex: 1, overflowY: "auto", padding: "22px 26px", display: "flex", flexDirection: "column", gap: 18 }}>{children}</div>
        <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "18px 26px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>{footer}</div>
      </div>
    </div>
  );
}

function labelField(label: string, node: ReactElement, req?: boolean): ReactElement {
  return (
    <label style={{ display: "block" }}>
      <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 3 }}>{label}{req === true && <b style={{ color: "var(--red)" }}> *</b>}</span>
      <div style={{ marginTop: 8 }}>{node}</div>
    </label>
  );
}

export interface AdminScreenProps {
  onBack: () => void;
}

export function AdminScreen({ onBack }: AdminScreenProps): ReactElement {
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<"users" | "roles">("users");
  const [q, setQ] = useState("");
  const [addUser, setAddUser] = useState(false);
  // With the identity provider on, adding somebody is an INVITATION: no
  // administrator invents a password, and the account only becomes usable once
  // its owner opens the link — which is also the proof they read mail there.
  const invites = providerLoginEnabled();
  const [addRole, setAddRole] = useState(false);
  const [editRoles, setEditRoles] = useState<AdminUser | null>(null);
  const [editPerms, setEditPerms] = useState<Role | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [handover, setHandover] = useState<{ user: AdminUser; invitation: Invitation } | null>(null);
  const [copied, setCopied] = useState(false);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const say = (m: string): void => { setToast(m); setTimeout(() => setToast(null), 2600); };

  const reload = useCallback((): void => {
    setLoadError(undefined);
    Promise.all([listUsers(API_URL), listRoles(API_URL)]).then(([us, rs]) => { setUsers(us); setRoles(rs); }).catch(() => setLoadError("No se pudo cargar usuarios y roles. Reintenta.")).finally(() => setLoading(false));
  }, []);
  useEffect(() => reload(), [reload]);

  const roleName = useCallback((id: string): string => roles.find((r) => r.id === id)?.name ?? id, [roles]);
  const stats = useMemo(() => ({ users: users.length, roles: roles.length, active: users.filter((u) => u.status === "active").length, suspended: users.filter((u) => u.status === "suspended").length, invited: users.filter((u) => u.status === "invited").length }), [users, roles]);

  const needle = q.trim().toLowerCase();
  const usersF = users.filter((u) => needle === "" || u.displayName.toLowerCase().includes(needle) || u.email.toLowerCase().includes(needle));
  const rolesF = roles.filter((r) => needle === "" || r.name.toLowerCase().includes(needle));

  const onCreateUser = async (input: CreateUserInput): Promise<void> => { await createUser(API_URL, input); setAddUser(false); reload(); say(`Usuario creado · ${input.displayName}`); };
  const onInviteUser = async (input: InviteUserInput): Promise<void> => {
    const created = await inviteUser(API_URL, input);
    setAddUser(false);
    reload();
    if (created.invitation.sent) {
      say(`Invitación enviada · ${input.email}`);
      return;
    }
    // The account exists and the message did not go out. Saying "invitación
    // enviada" here is the exact sentence that made this bug invisible.
    setHandover({ user: created, invitation: created.invitation });
  };
  const onCreateRole = async (input: CreateRoleInput): Promise<void> => { await createRole(API_URL, input); setAddRole(false); reload(); say(`Rol creado · ${input.name}`); };
  const onSuspend = async (u: AdminUser): Promise<void> => { await setUserStatus(API_URL, u.id, u.status === "active" ? "suspended" : "active"); reload(); say(u.status === "active" ? "Usuario suspendido" : "Usuario reactivado"); };
  /**
   * Resending, and telling the truth about it.
   *
   * The old version said "Enlace enviado" on success and "No se pudo enviar el
   * enlace" on any failure at all — including an expired session, which it
   * reported as a mail problem. Both halves were wrong in the same way: they
   * described what we HOPED happened instead of what did.
   *
   * Now the outcome decides. Delivered: a toast, done. Not delivered: the link
   * stays on screen with the reason, because a director who cannot mail somebody
   * can still hand them the link.
   */
  const onResend = async (u: AdminUser): Promise<void> => {
    try {
      const inv = await resendInvitation(API_URL, u.id);
      if (inv.sent) {
        say(`Invitación enviada · ${u.email}`);
        return;
      }
      setHandover({ user: u, invitation: inv });
    } catch (e) {
      // A dead session is not a mail problem, and saying so sent the director
      // looking at the mail server for an hour. Same lesson as
      // blanket-catch-reports-dead-session-as-domain-error.
      const dead = e instanceof Error && e.message.includes("401");
      say(dead ? "Tu sesión caducó. Vuelve a entrar." : "No se pudo generar la invitación");
    }
  };
  const onSaveUserRoles = async (u: AdminUser, roleIds: string[]): Promise<void> => { try { await setUserRoles(API_URL, u.id, roleIds); setEditRoles(null); reload(); say("Roles actualizados"); } catch { say("No se pudieron guardar los roles"); } };
  const onSavePerms = async (r: Role, permissions: string[]): Promise<void> => { try { await setRolePermissions(API_URL, r.id, permissions); setEditPerms(null); reload(); say("Permisos actualizados"); } catch { say("No se pudieron guardar los permisos"); } };

  const statCards: { icon: IconName; c: string; label: string; val: string; sub: string }[] = [
    { icon: "user", c: "forest", label: "Usuarios", val: String(stats.users), sub: "Cuentas del sistema" },
    { icon: "shield", c: "lime", label: "Roles", val: String(stats.roles), sub: "Conjuntos de permisos" },
    { icon: "check", c: "purple", label: "Activos", val: String(stats.active), sub: "Con acceso" },
    { icon: "warning", c: "red", label: "Suspendidos", val: String(stats.suspended), sub: "Sin acceso" },
  ];

  return (
    <div style={{ maxWidth: 1320, margin: "0 auto", padding: "24px 28px 96px" }}>
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}><Icon name="back" size={18} stroke={2.4} />Volver al inicio</button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 16, marginBottom: 24 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--forest) 38%, transparent)" }}><Icon name="shield" size={30} /></div>
          <div><h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Administración</h1><p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Usuarios, roles y permisos</p></div>
        </div>
        <div style={{ display: "flex", gap: 12 }}>
          {tab === "users" ? <Btn icon="plus" onClick={() => setAddUser(true)} style={{ height: 52, fontSize: 16.5 }}>Nuevo usuario</Btn> : <Btn icon="plus" onClick={() => setAddRole(true)} style={{ height: 52, fontSize: 16.5 }}>Nuevo rol</Btn>}
        </div>
      </div>

      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 26 }}>
        {statCards.map((k, i) => (
          <div key={i} style={{ flex: "1 1 200px", minWidth: 0, display: "flex", alignItems: "center", gap: 14, padding: "18px 20px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
            <div style={{ width: 46, height: 46, borderRadius: 13, flexShrink: 0, display: "grid", placeItems: "center", background: `color-mix(in oklab, var(--${k.c}) 16%, var(--surface))`, color: `var(--${k.c})` }}><Icon name={k.icon} size={24} /></div>
            <div style={{ minWidth: 0 }}><div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600 }}>{k.label}</div><div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 22, color: "var(--text)", lineHeight: 1.2 }}>{k.val}</div><div style={{ fontSize: 12, color: "var(--text-faint)" }}>{k.sub}</div></div>
          </div>
        ))}
      </div>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap", marginBottom: 18 }}>
        <div style={{ display: "flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)" }}>
          {([["users", "Usuarios", stats.users], ["roles", "Roles", stats.roles]] as const).map(([id, label, n]) => {
            const on = tab === id;
            return <button key={id} onClick={() => setTab(id)} style={{ display: "inline-flex", alignItems: "center", gap: 8, height: 40, padding: "0 16px", borderRadius: 99, cursor: "pointer", background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 600, fontSize: 14, fontFamily: "var(--font-display)", boxShadow: on ? "var(--shadow-card)" : "none" }}>{label}<span style={{ minWidth: 22, height: 20, padding: "0 6px", borderRadius: 99, background: on ? "var(--surface-3)" : "var(--surface)", color: "var(--text-muted)", fontSize: 12, display: "grid", placeItems: "center" }}>{n}</span></button>;
          })}
        </div>
        <div style={{ position: "relative", flex: 1, minWidth: 230, maxWidth: 360 }}>
          <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}><Icon name="search" size={19} /></span>
          <input value={q} onChange={(e) => setQ(e.target.value)} aria-label="Buscar" placeholder="Buscar…" style={{ width: "100%", height: 48, paddingLeft: 46, paddingRight: 16, borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 15, outline: "none" }} />
        </div>
      </div>

      <div className="fade-up" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
        {loading ? <div style={{ padding: 56, textAlign: "center", color: "var(--text-muted)" }}>Cargando…</div> : loadError !== undefined ? (
          <div role="alert" style={{ padding: 56, textAlign: "center" }}><div style={{ color: "var(--bad-fg)", fontSize: 15, fontWeight: 600 }}>{loadError}</div><div style={{ marginTop: 16 }}><Btn kind="ghost" icon="check" onClick={reload}>Reintentar</Btn></div></div>
        ) : tab === "users" ? (
          <div style={{ padding: 8 }}>
            {usersF.map((u, i) => (
              <div key={u.id} style={{ display: "grid", gridTemplateColumns: "2.4fr 1.6fr 1fr auto", alignItems: "center", gap: 16, padding: "13px 24px", borderRadius: "var(--r-md)" }}>
                <div style={{ display: "flex", alignItems: "center", gap: 13, minWidth: 0 }}>
                  <Avatar name={u.displayName} color={AV[i % AV.length]} size={42} />
                  <div style={{ minWidth: 0 }}><div style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)" }}>{u.displayName}</div><div style={{ fontSize: 12.5, color: "var(--text-faint)" }}>{u.email}</div></div>
                </div>
                <div style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{(u.roleIds ?? []).length > 0 ? (u.roleIds ?? []).map(roleName).join(", ") : "Sin roles"}</div>
                <div><StatusPill status={u.status} /></div>
                <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
                  <button onClick={() => setEditRoles(u)} style={{ height: 38, padding: "0 14px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, color: "var(--forest)", background: "var(--surface-3)", border: "1px solid var(--border)", cursor: "pointer" }}>Roles</button>
                  {/* ONLY for somebody who has not walked through the door
                      yet. It used to render on everybody, including accounts in
                      daily use, which is what the owner reported. The backend
                      refuses those too — this is the courtesy, that is the rule. */}
                  {invites && u.status !== "active" && <button onClick={() => void onResend(u)} style={{ height: 38, padding: "0 14px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, color: "var(--forest)", background: "var(--surface-3)", border: "1px solid var(--border)", cursor: "pointer" }}>Reenviar invitación</button>}
                  <button onClick={() => void onSuspend(u)} style={{ height: 38, padding: "0 14px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, color: u.status === "active" ? "var(--bad-fg)" : "var(--forest)", background: "var(--surface-3)", border: "1px solid var(--border)", cursor: "pointer" }}>{u.status === "active" ? "Suspender" : "Reactivar"}</button>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div style={{ padding: 8 }}>
            {rolesF.map((r) => (
              <div key={r.id} style={{ display: "grid", gridTemplateColumns: "1.6fr 2fr auto", alignItems: "center", gap: 16, padding: "15px 24px", borderRadius: "var(--r-md)" }}>
                <div style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)", textTransform: "capitalize" }}>{r.name}</div>
                <div style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{(r.permissions ?? []).length} permiso(s)</div>
                <div style={{ display: "flex", justifyContent: "flex-end" }}><button onClick={() => setEditPerms(r)} style={{ height: 38, padding: "0 14px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, color: "var(--forest)", background: "var(--surface-3)", border: "1px solid var(--border)", cursor: "pointer" }}>Editar permisos</button></div>
              </div>
            ))}
          </div>
        )}
      </div>

      {addUser && (invites
        ? <InviteUserDrawer roles={roles} onClose={() => setAddUser(false)} onSave={onInviteUser} />
        : <CreateUserDrawer roles={roles} onClose={() => setAddUser(false)} onSave={onCreateUser} />)}
      {addRole && <CreateRoleDrawer onClose={() => setAddRole(false)} onSave={onCreateRole} />}
      {editRoles !== null && <EditRolesDrawer user={editRoles} roles={roles} onClose={() => setEditRoles(null)} onSave={(ids) => onSaveUserRoles(editRoles, ids)} />}
      {editPerms !== null && <EditPermsDrawer role={editPerms} onClose={() => setEditPerms(null)} onSave={(ps) => onSavePerms(editPerms, ps)} />}
      {handover !== null && (
        <HandoverDrawer
          user={handover.user}
          invitation={handover.invitation}
          copied={copied}
          onCopy={() => { void navigator.clipboard.writeText(handover.invitation.url).then(() => setCopied(true)); }}
          onClose={() => { setHandover(null); setCopied(false); }}
        />
      )}
      {toast !== null && <div style={{ position: "fixed", bottom: 28, left: "50%", transform: "translateX(-50%)", zIndex: 80, display: "flex", alignItems: "center", gap: 11, padding: "14px 22px", borderRadius: "var(--r-pill)", background: "var(--forest)", color: "#fff", fontWeight: 600, fontSize: 15, boxShadow: "var(--shadow-pop)", animation: "pop .25s var(--ease) both" }}><Icon name="check" size={20} stroke={2.6} />{toast}</div>}
    </div>
  );
}

function CreateUserDrawer({ roles, onClose, onSave }: { roles: readonly Role[]; onClose: () => void; onSave: (i: CreateUserInput) => Promise<void> }): ReactElement {
  const [email, setEmail] = useState("");
  const [displayName, setName] = useState("");
  const [tempPassword, setPw] = useState("");
  const [sel, setSel] = useState<string[]>([]);
  const [err, setErr] = useState<string | undefined>(undefined);
  const ready = email.includes("@") && displayName.trim().length > 1 && tempPassword.length >= 6;
  const toggle = (id: string): void => setSel((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  const submit = async (): Promise<void> => { try { await onSave({ email: email.trim(), displayName: displayName.trim(), tempPassword, roleIds: sel }); } catch { setErr("No se pudo crear el usuario (¿correo duplicado?)."); } };
  return (
    <Drawer title="Nuevo usuario" subtitle="Cuenta de acceso al sistema" icon="user" onClose={onClose}
      footer={<><Btn kind="ghost" onClick={onClose}>Cancelar</Btn><div style={{ flex: 1 }} /><Btn icon="check" onClick={() => void submit()} style={{ opacity: ready ? 1 : 0.45, pointerEvents: ready ? "auto" : "none", height: 52 }}>Crear usuario</Btn></>}>
      <>
        {labelField("Correo", <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="nombre@garcialorca.edu.mx" style={fi} />, true)}
        {labelField("Nombre para mostrar", <input value={displayName} onChange={(e) => setName(e.target.value)} placeholder="Nombre del usuario" style={fi} />, true)}
        {labelField("Contraseña temporal", <input value={tempPassword} onChange={(e) => setPw(e.target.value)} type="password" placeholder="La cambiará en el primer ingreso" style={fi} />, true)}
        <div><span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 3 }}>Roles</span>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 8 }}>{roles.map((r) => <button key={r.id} onClick={() => toggle(r.id)} style={chip(sel.includes(r.id), "lime")}>{r.name}</button>)}</div>
        </div>
        {err !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{err}</p>}
      </>
    </Drawer>
  );
}

/**
 * The invitation drawer. Same fields as CreateUserDrawer MINUS the password —
 * that absence is the whole change, so it is stated rather than left to be
 * noticed: the person chooses their own first password from the emailed link,
 * and nobody at the school ever knows it.
 */
function InviteUserDrawer({ roles, onClose, onSave }: { roles: readonly Role[]; onClose: () => void; onSave: (i: InviteUserInput) => Promise<void> }): ReactElement {
  const [email, setEmail] = useState("");
  const [displayName, setName] = useState("");
  const [sel, setSel] = useState<string[]>([]);
  const [err, setErr] = useState<string | undefined>(undefined);
  const [sending, setSending] = useState(false);
  const ready = email.includes("@") && displayName.trim().length > 1 && !sending;
  const toggle = (id: string): void => setSel((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  const submit = async (): Promise<void> => {
    setSending(true);
    try { await onSave({ email: email.trim(), displayName: displayName.trim(), roleIds: sel }); }
    // The account may well exist and only the mail have failed, so the message
    // says what to do next instead of implying nothing happened.
    catch { setErr("No se pudo enviar la invitación (¿correo duplicado?). Verifica el correo y vuelve a intentar."); }
    finally { setSending(false); }
  };
  return (
    <Drawer title="Invitar usuario" subtitle="Recibirá un correo para elegir su contraseña" icon="user" onClose={onClose}
      footer={<><Btn kind="ghost" onClick={onClose}>Cancelar</Btn><div style={{ flex: 1 }} /><Btn icon="check" onClick={() => void submit()} style={{ opacity: ready ? 1 : 0.45, pointerEvents: ready ? "auto" : "none", height: 52 }}>{sending ? "Enviando…" : "Enviar invitación"}</Btn></>}>
      <>
        {labelField("Correo", <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="nombre@garcialorca.edu.mx" style={fi} />, true)}
        {labelField("Nombre para mostrar", <input value={displayName} onChange={(e) => setName(e.target.value)} placeholder="Nombre del usuario" style={fi} />, true)}
        <div><span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 3 }}>Roles</span>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 8 }}>{roles.map((r) => <button key={r.id} onClick={() => toggle(r.id)} style={chip(sel.includes(r.id), "lime")}>{r.name}</button>)}</div>
        </div>
        <p style={{ fontSize: 13.5, color: "var(--text-muted)", margin: 0, lineHeight: 1.5 }}>
          No se define ninguna contraseña aquí: la persona elegirá la suya desde el enlace que reciba. Si no le llega, usa «Reenviar acceso» en su renglón de la tabla.
        </p>
        {err !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{err}</p>}
      </>
    </Drawer>
  );
}

function CreateRoleDrawer({ onClose, onSave }: { onClose: () => void; onSave: (i: CreateRoleInput) => Promise<void> }): ReactElement {
  const [name, setName] = useState("");
  const [sel, setSel] = useState<string[]>([]);
  const [err, setErr] = useState<string | undefined>(undefined);
  const toggle = (v: string): void => setSel((s) => (s.includes(v) ? s.filter((x) => x !== v) : [...s, v]));
  const submit = async (): Promise<void> => { try { await onSave({ name: name.trim(), permissions: sel }); } catch { setErr("No se pudo crear el rol."); } };
  return (
    <Drawer title="Nuevo rol" subtitle="Conjunto de permisos" icon="shield" onClose={onClose}
      footer={<><Btn kind="ghost" onClick={onClose}>Cancelar</Btn><div style={{ flex: 1 }} /><Btn icon="check" onClick={() => void submit()} style={{ opacity: name.trim().length > 1 ? 1 : 0.45, pointerEvents: name.trim().length > 1 ? "auto" : "none", height: 52 }}>Crear rol</Btn></>}>
      <>
        {labelField("Nombre del rol", <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Coordinación, Docente…" style={fi} />, true)}
        <PermChecklist selected={sel} onToggle={toggle} />
        {err !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{err}</p>}
      </>
    </Drawer>
  );
}

function EditRolesDrawer({ user, roles, onClose, onSave }: { user: AdminUser; roles: readonly Role[]; onClose: () => void; onSave: (ids: string[]) => Promise<void> }): ReactElement {
  const [sel, setSel] = useState<string[]>(user.roleIds ?? []);
  const toggle = (id: string): void => setSel((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  return (
    <Drawer title={`Roles de ${user.displayName}`} subtitle={user.email} icon="user" onClose={onClose}
      footer={<><Btn kind="ghost" onClick={onClose}>Cancelar</Btn><div style={{ flex: 1 }} /><Btn icon="check" onClick={() => void onSave(sel)} style={{ height: 52 }}>Guardar roles</Btn></>}>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>{roles.map((r) => <button key={r.id} onClick={() => toggle(r.id)} style={chip(sel.includes(r.id), "lime")}>{r.name}</button>)}</div>
    </Drawer>
  );
}

function EditPermsDrawer({ role, onClose, onSave }: { role: Role; onClose: () => void; onSave: (ps: string[]) => Promise<void> }): ReactElement {
  const [sel, setSel] = useState<string[]>(role.permissions ?? []);
  const toggle = (v: string): void => setSel((s) => (s.includes(v) ? s.filter((x) => x !== v) : [...s, v]));
  return (
    <Drawer title={`Permisos de ${role.name}`} subtitle="Marca lo que este rol puede hacer" icon="shield" onClose={onClose}
      footer={<><Btn kind="ghost" onClick={onClose}>Cancelar</Btn><div style={{ flex: 1 }} /><Btn icon="check" onClick={() => void onSave(sel)} style={{ height: 52 }}>Guardar permisos</Btn></>}>
      <PermChecklist selected={sel} onToggle={toggle} />
    </Drawer>
  );
}

function PermChecklist({ selected, onToggle }: { selected: string[]; onToggle: (v: string) => void }): ReactElement {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      {PERMISSION_CATALOG.map((g) => (
        <div key={g.resource}>
          <div style={{ fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em", marginBottom: 8 }}>{g.resource}</div>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            {g.permissions.map((p) => <button key={p.value} aria-label={p.label} onClick={() => onToggle(p.value)} style={chip(selected.includes(p.value), "lime")}>{p.label}</button>)}
          </div>
        </div>
      ))}
    </div>
  );
}

/**
 * What to do when the mail did not go out.
 *
 * This is the screen the whole change exists for. Before it, an undelivered
 * invitation produced a green toast saying "Invitación enviada" and nothing
 * else — so the director told somebody to check an inbox nothing had reached,
 * and the failure stayed invisible for a week.
 *
 * It does two things a toast cannot: it says WHY in words the director can act
 * on, and it hands over the link, which is the only way to give somebody access
 * while the school still has no mail server configured.
 */
function HandoverDrawer({ user, invitation, copied, onCopy, onClose }: { user: AdminUser; invitation: Invitation; copied: boolean; onCopy: () => void; onClose: () => void }): ReactElement {
  return (
    <Drawer
      title="El correo no salió"
      subtitle={`La cuenta de ${user.displayName} sí quedó creada`}
      icon="warning"
      onClose={onClose}
      footer={<Btn icon="check" onClick={onClose}>Entendido</Btn>}
    >
      <div style={{ display: "grid", gap: 18 }}>
        <div style={{ padding: "13px 15px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 14, lineHeight: 1.5 }}>
          {humanSendError(invitation.sendError)}
        </div>
        <div>
          <div style={{ fontSize: 14, fontWeight: 600, color: "var(--text-muted)", marginBottom: 8 }}>
            Pásale este enlace a {user.displayName}
          </div>
          <div style={{ padding: "12px 14px", borderRadius: "var(--r-md)", border: "1px solid var(--border)", background: "var(--surface-3)", fontSize: 12.5, wordBreak: "break-all", color: "var(--text)", fontFamily: "ui-monospace, monospace" }}>
            {invitation.url}
          </div>
          <div style={{ marginTop: 10 }}>
            <Btn kind="ghost" icon={copied ? "check" : "documents"} onClick={onCopy}>
              {copied ? "Copiado" : "Copiar enlace"}
            </Btn>
          </div>
        </div>
        <p style={{ fontSize: 13.5, color: "var(--text-faint)", lineHeight: 1.5 }}>
          {expiryPhrase(invitation.expiresAt)} Después deja de servir y tendrás que reenviar la invitación.
        </p>
      </div>
    </Drawer>
  );
}

/**
 * The mail server's own words are for whoever fixes the server. The director
 * needs the sentence that tells them what to do next.
 */
function humanSendError(raw: string | undefined): string {
  const text = raw ?? "";
  if (text === "") return "No pudimos enviar el correo.";
  if (text.includes("no hay un servidor de correo")) {
    return "Todavía no hay un servidor de correo configurado, así que el sistema no puede enviar nada por su cuenta. Entrégale el enlace tú.";
  }
  if (text.includes("rechazó nuestra contraseña")) {
    return "El servidor de correo del colegio rechazó nuestra contraseña. Hay que renovarla; mientras tanto, entrégale el enlace tú.";
  }
  if (text.includes("no respondió") || text.includes("contactar")) {
    return "El servidor de correo no respondió. Puede ser pasajero: entrégale el enlace y vuelve a intentar el envío más tarde.";
  }
  if (text.includes("rechazó esa dirección")) {
    return "El servidor de correo rechazó esa dirección. Revisa que esté bien escrita.";
  }
  return "No pudimos enviar el correo. Entrégale el enlace tú.";
}

/** The deadline as a person would say it, never as an ISO string. */
function expiryPhrase(iso: string): string {
  if (iso === "") return "El enlace tiene vigencia limitada.";
  const when = new Date(iso);
  if (Number.isNaN(when.getTime())) return "El enlace tiene vigencia limitada.";
  const hours = Math.round((when.getTime() - Date.now()) / 3_600_000);
  if (hours <= 0) return "El enlace ya venció.";
  if (hours <= 12) return `El enlace funciona durante las próximas ${hours} horas.`;
  return `El enlace funciona hasta el ${when.getDate()} de ${MONTHS[when.getMonth()]} a las ${String(when.getHours()).padStart(2, "0")}:${String(when.getMinutes()).padStart(2, "0")}.`;
}

const MONTHS = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"];

"use client";

import { useEffect, useState, type ReactElement } from "react";
import { Btn } from "../_design/Controls";
import { formatCents as money } from "../_lib/money";
import { Icon, type IconName } from "../_design/Icon";
import { COLOR, type ColorKey } from "../_design/colors";
import { MODULES, isModuleVisible, type ModuleDef, type UserProfile } from "../_design/data";
import { listCycles } from "../config/api";
import { listCharges } from "../finance/api";
import { listStudents } from "../students/api";

import { API_URL } from "../api-url";


function ModuleCard({ m, i, onOpen }: { m: ModuleDef; i: number; onOpen: (id: string) => void }): ReactElement {
  const [hov, setHov] = useState(false);
  const c = COLOR[m.color] ?? COLOR.neutral;
  const neutral = m.color === "neutral";
  return (
    <button
      onClick={() => onOpen(m.id)}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      className="fade-up"
      style={{
        position: "relative",
        textAlign: "left",
        display: "flex",
        flexDirection: "column",
        gap: 16,
        padding: "22px 22px 20px",
        borderRadius: "var(--r-lg)",
        minHeight: 172,
        background: hov ? `color-mix(in oklab, ${c.v} 9%, var(--surface))` : "var(--surface)",
        border: `1.5px solid ${hov ? `color-mix(in oklab, ${c.v} 45%, var(--border))` : "var(--border)"}`,
        boxShadow: hov ? "var(--shadow-hover)" : "var(--shadow-card)",
        transform: hov ? "translateY(-4px)" : "none",
        transition: "all .22s var(--ease)",
        animationDelay: `${i * 0.03}s`
      }}
    >
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between" }}>
        <div
          style={{
            width: 58,
            height: 58,
            borderRadius: 18,
            display: "grid",
            placeItems: "center",
            flexShrink: 0,
            background: neutral ? "var(--surface-3)" : c.v,
            color: neutral ? "var(--text-muted)" : c.ink,
            boxShadow: neutral ? "none" : `0 6px 16px color-mix(in oklab, ${c.v} 38%, transparent)`,
            transform: hov ? "scale(1.06) rotate(-3deg)" : "none",
            transition: "transform .25s var(--ease)"
          }}
        >
          <Icon name={m.icon} size={30} stroke={2.1} />
        </div>
        {m.badge !== null && m.badge > 0 && (
          <span
            style={{
              minWidth: 26,
              height: 26,
              padding: "0 8px",
              borderRadius: 99,
              background: "var(--red)",
              color: "#fff",
              fontFamily: "var(--font-display)",
              fontWeight: 700,
              fontSize: 13,
              display: "grid",
              placeItems: "center"
            }}
          >
            {m.badge}
          </span>
        )}
      </div>
      <div style={{ marginTop: "auto" }}>
        <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 18.5, color: "var(--text)", letterSpacing: "-.02em" }}>
          {m.label}
        </div>
        <div style={{ fontSize: 13.5, color: "var(--text-muted)", marginTop: 3 }}>{m.desc}</div>
      </div>
      <span
        style={{
          position: "absolute",
          right: 18,
          bottom: 18,
          color: c.v,
          opacity: hov ? 1 : 0,
          transform: hov ? "translateX(0)" : "translateX(-6px)",
          transition: "all .22s var(--ease)"
        }}
      >
        <Icon name="chevron" size={20} stroke={2.6} />
      </span>
    </button>
  );
}

function KpiCard({
  icon,
  color,
  label,
  value,
  sub,
  onClick
}: {
  icon: IconName;
  color: ColorKey;
  label: string;
  value: string;
  sub: string;
  onClick: () => void;
}): ReactElement {
  const [hov, setHov] = useState(false);
  const c = COLOR[color];
  return (
    <button
      onClick={onClick}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      style={{
        flex: 1,
        textAlign: "left",
        display: "flex",
        alignItems: "center",
        gap: 16,
        padding: "18px 22px",
        borderRadius: "var(--r-lg)",
        background: "var(--surface)",
        border: "1px solid var(--border)",
        boxShadow: hov ? "var(--shadow-hover)" : "var(--shadow-card)",
        transform: hov ? "translateY(-3px)" : "none",
        transition: "all .2s var(--ease)"
      }}
    >
      <div
        style={{
          width: 52,
          height: 52,
          borderRadius: 14,
          flexShrink: 0,
          display: "grid",
          placeItems: "center",
          background: `color-mix(in oklab, ${c.v} 16%, var(--surface))`,
          color: c.v
        }}
      >
        <Icon name={icon} size={26} />
      </div>
      <div>
        <div style={{ fontSize: 13.5, color: "var(--text-muted)", fontWeight: 600 }}>{label}</div>
        <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 26, color: "var(--text)", lineHeight: 1.1, marginTop: 2 }}>
          {value}
        </div>
        <div style={{ fontSize: 12.5, color: "var(--text-faint)", marginTop: 2 }}>{sub}</div>
      </div>
    </button>
  );
}

// Grid shape shared by the real cards, the skeleton, and the empty state so the
// modules section never reflows between loading and loaded.
const GRID_STYLE = { display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(236px, 1fr))", gap: 18 } as const;

// Loading placeholder shown while `me()` is still resolving — avoids flashing
// every module and then hiding the ones the caller cannot read.
function ModulesSkeleton(): ReactElement {
  return (
    <div role="status" aria-label="Cargando módulos" style={GRID_STYLE}>
      {Array.from({ length: 6 }, (_, i) => (
        <div
          key={i}
          aria-hidden
          style={{
            minHeight: 172,
            borderRadius: "var(--r-lg)",
            background: "var(--surface)",
            border: "1px solid var(--border)",
            opacity: 0.55
          }}
        />
      ))}
    </div>
  );
}

// Shown when the caller's permissions could not be loaded.
//
// NOT the same as "you have no modules", and saying that would be a lie: the
// system does not know what this person can do, which is a fault on our side.
// The previous behaviour said nothing at all — it kept showing the loading
// skeleton — and a session broken this way was indistinguishable from a slow
// network for as long as anybody cared to wait.
function PermissionsUnavailable({ onRetry }: { onRetry?: () => void }): ReactElement {
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

// Shown to a signed-in user whose permissions match no module at all.
function NoModules(): ReactElement {
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
        No tienes módulos asignados
      </div>
      <div style={{ fontSize: 13.5 }}>Solicita acceso a tu administrador para ver tus módulos aquí.</div>
    </div>
  );
}

export interface DashboardProps {
  user: UserProfile;
  onOpen: (id: string) => void;
  /**
   * The authenticated principal's READ permissions. Drives which module tiles
   * and KPI shortcuts are visible. `undefined` until `me()` resolves — the grid
   * shows a skeleton rather than flashing every module. UX-only; the gateway is
   * the security boundary.
   */
  permissions?: readonly string[];
  /**
   * The permissions could not be loaded at all (the /me call failed).
   *
   * Distinct from `permissions === undefined`, and the distinction is the whole
   * point: undefined alone rendered the loading skeleton, so a failed call
   * looked exactly like a slow one and shimmered forever. The owner read that
   * as "no hay módulos" and reported it as such (dev, 2026-09-07).
   */
  permissionsFailed?: boolean;
  /** Retry loading them, so the only way out is not signing out again. */
  onRetryPermissions?: () => void;
  /** Injectable clock so the greeting/date are deterministic under test. */
  now?: Date;
}

export function Dashboard({ user, onOpen, permissions, permissionsFailed = false, onRetryPermissions, now = new Date() }: DashboardProps): ReactElement {
  const hour = now.getHours();
  const greet = hour < 12 ? "Buenos días" : hour < 19 ? "Buenas tardes" : "Buenas noches";
  const today = now.toLocaleDateString("es-MX", { weekday: "long", day: "numeric", month: "long" });

  // Role-aware visibility: a tile/shortcut is shown IFF the principal holds the
  // module's READ permission. `undefined` means `me()` has not resolved yet.
  const resolved = permissions !== undefined;
  const visibleModules = resolved ? MODULES.filter((m) => isModuleVisible(m, permissions)) : [];
  const canSee = (perm: string): boolean => resolved && permissions.includes(perm);

  // Real KPIs — no mock numbers: students from the roster, money from the ledger.
  const [kpi, setKpi] = useState({ students: 0, pending: 0, collected: 0, withBalance: 0 });
  const [cycleName, setCycleName] = useState("—");
  useEffect(() => {
    let active = true;
    // "Registrados": the active admission pipeline — exclude rejected/withdrawn,
    // which are not current students, so the count never over-reports.
    listStudents(API_URL).then((ss) => active && setKpi((k) => ({ ...k, students: ss.filter((s) => s.admissionStatus !== "rejected" && s.admissionStatus !== "withdrawn").length }))).catch(() => {});
    listCycles(API_URL).then((cs) => {
      const cyc = cs.find((c) => c.status === "active") ?? cs[0];
      if (cyc === undefined) return;
      if (active) setCycleName(cyc.name);
      listCharges(API_URL, { cycleId: cyc.id }).then((chs) => {
        if (!active) return;
        setKpi((k) => ({
          ...k,
          collected: chs.reduce((a, c) => a + c.paidCents, 0),
          pending: chs.reduce((a, c) => a + Math.max(0, c.remainingCents), 0),
          withBalance: chs.filter((c) => c.remainingCents > 0).length,
        }));
      }).catch(() => {});
    }).catch(() => {});
    return () => { active = false; };
  }, []);

  return (
    <div style={{ maxWidth: 1320, margin: "0 auto", padding: "34px 28px 64px" }}>
      {/* welcome */}
      <div
        className="fade-up"
        style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", flexWrap: "wrap", gap: 16, marginBottom: 24 }}
      >
        <div>
          <h1 style={{ fontSize: 34, fontWeight: 800, color: "var(--text)" }}>
            {greet}, {user.first} 🌱
          </h1>
          <p style={{ color: "var(--text-muted)", fontSize: 16, marginTop: 6, textTransform: "capitalize" }}>
            {today} · Ciclo {cycleName}
          </p>
        </div>
        {canSee("comms:read") && (
          <Btn icon="plus" kind="forest" onClick={() => onOpen("announcements")}>
            Nuevo aviso
          </Btn>
        )}
      </div>

      {/* KPIs — each shortcut is shown only to callers who can read its module. */}
      {(canSee("students:read") || canSee("finance:read")) && (
        <div className="fade-up" style={{ display: "flex", gap: 18, flexWrap: "wrap", marginBottom: 34 }}>
          {canSee("students:read") && (
            <KpiCard icon="students" color="forest" label="Alumnos" value={String(kpi.students)} sub="Registrados este ciclo" onClick={() => onOpen("students")} />
          )}
          {canSee("finance:read") && (
            <>
              <KpiCard icon="finance" color="orange" label="Por cobrar" value={money(kpi.pending)} sub={`${kpi.withBalance} cargos con saldo`} onClick={() => onOpen("finance")} />
              <KpiCard icon="trend" color="lime" label="Cobrado" value={money(kpi.collected)} sub={`Ciclo ${cycleName}`} onClick={() => onOpen("finance")} />
            </>
          )}
        </div>
      )}

      {/* modules */}
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", marginBottom: 16 }}>
        <h2 style={{ fontSize: 21, fontWeight: 700, color: "var(--text)" }}>Módulos</h2>
        <span style={{ fontSize: 14, color: "var(--text-faint)" }}>Todo el sistema, a la vista</span>
      </div>
      {permissionsFailed ? (
        <PermissionsUnavailable onRetry={onRetryPermissions} />
      ) : !resolved ? (
        <ModulesSkeleton />
      ) : visibleModules.length === 0 ? (
        <NoModules />
      ) : (
        <div style={GRID_STYLE}>
          {visibleModules.map((m, i) => (
            <ModuleCard key={m.id} m={m} i={i} onOpen={onOpen} />
          ))}
        </div>
      )}
    </div>
  );
}

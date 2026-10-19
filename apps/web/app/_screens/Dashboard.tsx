"use client";

import { useFlags } from "@garcia-lorca/ui/flags";
import type { CSSProperties, ReactElement } from "react";
import { Btn } from "../_design/Controls";
import { formatCents as money } from "../_lib/money";
import { Icon, type IconName } from "../_design/Icon";
import { Page } from "../_design/Page";
import { COLOR, type ColorKey } from "../_design/colors";
import { MODULES, isModuleEnabled, isModuleVisible, type ModuleDef } from "../_design/data";
import { useFlagsResolved } from "../flags-resolved";
import { greetingFor, todayLabel } from "../home/greeting";
import { HomeView } from "../home/HomeView";
import { NoModules, PermissionsUnavailable } from "../home/HomeNotices";
import { useRefreshedHome } from "../home/homeVariant";
import type { HomeProps } from "../home/types";
import { useHomeKpis, type Ledger } from "../home/useHomeKpis";

const NO_LEDGER: Ledger = { pendingCents: 0, collectedCents: 0, withBalance: 0, charges: 0 };

/** CSS custom properties are the one inline style a card keeps: they feed its classes. */
type CardVars = CSSProperties & Record<`--${string}`, string>;

// Every style lives in globals.css (.module-card*, .kpi-card*), hover
// included (:hover). An inline style would beat the look layer under
// html[data-look]; only the module's color and the stagger stay inline, as
// custom properties.
function ModuleCard({ m, i, onOpen }: { m: ModuleDef; i: number; onOpen: (id: string) => void }): ReactElement {
  const c = COLOR[m.color] ?? COLOR.neutral;
  const neutral = m.color === "neutral";
  const vars: CardVars = { "--mod-c": c.v, "--mod-ink": c.ink, "--card-delay": `${i * 0.03}s` };
  return (
    <button onClick={() => onOpen(m.id)} className={neutral ? "module-card module-card--neutral fade-up" : "module-card fade-up"} style={vars}>
      <div className="module-card-top">
        <div className="module-card-icon">
          <Icon name={m.icon} size={30} stroke={2.1} />
        </div>
        {m.badge !== null && m.badge > 0 && <span className="module-card-badge">{m.badge}</span>}
      </div>
      <div className="module-card-text">
        <div className="module-card-label">{m.label}</div>
        <div className="module-card-desc">{m.desc}</div>
      </div>
      <span className="module-card-chevron">
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
  const vars: CardVars = { "--kpi-c": COLOR[color].v };
  return (
    <button onClick={onClick} className="kpi-card" style={vars}>
      <div className="kpi-card-icon">
        <Icon name={icon} size={26} />
      </div>
      <div>
        <div className="kpi-card-label">{label}</div>
        <div className="kpi-card-value">{value}</div>
        <div className="kpi-card-sub">{sub}</div>
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

export type DashboardProps = HomeProps;

/** The home: today's, or the refreshed one when module_ui_refresh is on. */
export function Dashboard(props: DashboardProps): ReactElement {
  return useRefreshedHome() ? <HomeView {...props} /> : <ClassicDashboard {...props} />;
}

function ClassicDashboard({ user, onOpen, permissions, permissionsFailed = false, onRetryPermissions, now = new Date() }: DashboardProps): ReactElement {
  const greet = greetingFor(now);
  const today = todayLabel(now);

  // Role-aware visibility: a tile/shortcut is shown IFF the module is switched
  // on and the principal holds its READ permission. `undefined` means `me()`
  // has not resolved yet.
  const flags = useFlags();
  const resolved = permissions !== undefined;
  // Until /features answers every flag reads OFF: the grid waits for both.
  const flagsResolved = useFlagsResolved();
  const modulesReady = resolved && flagsResolved;
  const visibleModules = modulesReady ? MODULES.filter((m) => isModuleVisible(m, permissions) && isModuleEnabled(m, flags)) : [];
  const canSee = (perm: string): boolean => resolved && permissions.includes(perm);

  // Real KPIs — no mock numbers: students from the roster, money from the ledger.
  // Each read runs only for a caller who may make it: a maestro or a cajera has
  // no students:read / config:read, and reading anyway filled their console
  // with 403s on the one screen everybody opens. This view reads every state
  // as a plain number (0 until it is known), as it always has.
  const readsCycles = canSee("config:read");
  const { students, cycle, ledger } = useHomeKpis({
    students: canSee("students:read"),
    cycles: readsCycles,
    ledger: readsCycles && canSee("finance:read"),
  });
  const ledgerNow = ledger.status === "ready" ? ledger.value : NO_LEDGER;
  const kpi = {
    students: students.status === "ready" ? students.value : 0,
    pending: ledgerNow.pendingCents,
    collected: ledgerNow.collectedCents,
    withBalance: ledgerNow.withBalance,
  };
  const cycleName = cycle?.name;

  return (
    <Page>
      {/* welcome */}
      <div
        className="fade-up"
        style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", flexWrap: "wrap", gap: 16, marginBottom: 24 }}
      >
        <div>
          <h1 style={{ fontSize: 34, fontWeight: 800, color: "var(--text)" }}>
            {greet}, {user.first} 🌱
          </h1>
          <p style={{ color: "var(--text-muted)", fontSize: 16, marginTop: 6 }}>
            {today}{cycleName !== undefined && ` · Ciclo ${cycleName}`}
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
              <KpiCard icon="trend" color="lime" label="Cobrado" value={money(kpi.collected)} sub={`Ciclo ${cycleName ?? "—"}`} onClick={() => onOpen("finance")} />
            </>
          )}
        </div>
      )}

      {/* modules — no heading over nothing: NoModules says it on its own (CC-315) */}
      {!(modulesReady && !permissionsFailed && visibleModules.length === 0) && (
        <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", marginBottom: 16 }}>
          <h2 style={{ fontSize: 21, fontWeight: 700, color: "var(--text)" }}>Módulos</h2>
          <span style={{ fontSize: 14, color: "var(--text-faint)" }}>Todo el sistema, a la vista</span>
        </div>
      )}
      {permissionsFailed ? (
        <PermissionsUnavailable onRetry={onRetryPermissions} />
      ) : !modulesReady ? (
        <ModulesSkeleton />
      ) : visibleModules.length === 0 ? (
        <NoModules permissions={permissions} />
      ) : (
        <div style={GRID_STYLE}>
          {visibleModules.map((m, i) => (
            <ModuleCard key={m.id} m={m} i={i} onOpen={onOpen} />
          ))}
        </div>
      )}
    </Page>
  );
}

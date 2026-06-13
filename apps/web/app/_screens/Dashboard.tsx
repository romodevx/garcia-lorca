"use client";

import { useState, type ReactElement } from "react";
import { Btn } from "../_design/Controls";
import { Icon, type IconName } from "../_design/Icon";
import { COLOR, type ColorKey } from "../_design/colors";
import { MODULES, type ModuleDef, type UserProfile } from "../_design/data";

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

export interface DashboardProps {
  user: UserProfile;
  onOpen: (id: string) => void;
  /** Injectable clock so the greeting/date are deterministic under test. */
  now?: Date;
}

export function Dashboard({ user, onOpen, now = new Date() }: DashboardProps): ReactElement {
  const hour = now.getHours();
  const greet = hour < 12 ? "Buenos días" : hour < 19 ? "Buenas tardes" : "Buenas noches";
  const today = now.toLocaleDateString("es-MX", { weekday: "long", day: "numeric", month: "long" });
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
            {today} · Ciclo 2025–2026
          </p>
        </div>
        <Btn icon="plus" kind="forest" onClick={() => onOpen("announcements")}>
          Nuevo aviso
        </Btn>
      </div>

      {/* KPIs */}
      <div className="fade-up" style={{ display: "flex", gap: 18, flexWrap: "wrap", marginBottom: 34 }}>
        <KpiCard icon="qr" color="lime" label="Asistencia de hoy" value="92%" sub="1,148 de 1,248 presentes" onClick={() => onOpen("attendance")} />
        <KpiCard icon="finance" color="orange" label="Adeudos del mes" value="$128,300" sub="37 alumnos pendientes" onClick={() => onOpen("finance")} />
        <KpiCard icon="announcements" color="purple" label="Avisos nuevos" value="2" sub="Festival de primavera" onClick={() => onOpen("announcements")} />
      </div>

      {/* modules */}
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", marginBottom: 16 }}>
        <h2 style={{ fontSize: 21, fontWeight: 700, color: "var(--text)" }}>Módulos</h2>
        <span style={{ fontSize: 14, color: "var(--text-faint)" }}>Todo el sistema, a la vista</span>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(236px, 1fr))", gap: 18 }}>
        {MODULES.map((m, i) => (
          <ModuleCard key={m.id} m={m} i={i} onOpen={onOpen} />
        ))}
      </div>
    </div>
  );
}

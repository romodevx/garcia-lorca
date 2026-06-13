"use client";

import { useState, type CSSProperties, type ReactElement } from "react";
import { Emblem } from "../_design/Brand";
import { Btn, ThemeToggle } from "../_design/Controls";
import { Icon, type IconName } from "../_design/Icon";
import type { Role } from "../_design/data";
import type { Theme } from "../theme/theme-script";

export interface LoginProps {
  theme: Theme;
  onToggle: () => void;
  onLogin: (role: Role) => void;
}

const inp: CSSProperties = {
  width: "100%",
  height: 52,
  paddingLeft: 46,
  paddingRight: 16,
  borderRadius: "var(--r-md)",
  border: "1.5px solid var(--border)",
  background: "var(--surface)",
  color: "var(--text)",
  fontSize: 16,
  outline: "none"
};

interface RoleOption {
  id: Role;
  label: string;
  icon: IconName;
}

const ROLES: readonly RoleOption[] = [
  { id: "admin", label: "Administración", icon: "shield" },
  { id: "teacher", label: "Maestro", icon: "graduation" }
];

const PLACEHOLDER_USER: Record<Role, string> = { admin: "director", teacher: "mvazquez" };

const ROSETTE_TOP: ReadonlyArray<[string, number]> = [
  ["var(--lime)", -90],
  ["var(--yellow)", -26],
  ["var(--orange)", 38],
  ["var(--purple)", 102],
  ["#fff", 166],
  ["var(--lime)", 230]
];
const ROSETTE_BOTTOM: ReadonlyArray<[string, number]> = [
  ["var(--yellow)", -90],
  ["var(--orange)", -26],
  ["var(--lime)", 38],
  ["#fff", 102],
  ["var(--purple)", 166],
  ["var(--lime)", 230]
];

function Rosette({ petals, style }: { petals: ReadonlyArray<[string, number]>; style: CSSProperties }): ReactElement {
  return (
    <svg viewBox="0 0 100 100" style={style} aria-hidden="true">
      {petals.map(([c, a], i) => {
        const r = (a * Math.PI) / 180;
        return <circle key={i} cx={50 + Math.cos(r) * 23} cy={50 + Math.sin(r) * 23} r={27} fill={c} />;
      })}
    </svg>
  );
}

export function Login({ theme, onToggle, onLogin }: LoginProps): ReactElement {
  const [role, setRole] = useState<Role>("admin");
  const [user, setUser] = useState("director");
  const [pwd, setPwd] = useState("••••••••");
  const [show, setShow] = useState(false);

  return (
    <div className="app-root" style={{ minHeight: "100vh", display: "grid", gridTemplateColumns: "1.05fr .95fr" }}>
      {/* brand panel */}
      <div
        style={{
          position: "relative",
          overflow: "hidden",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: "56px 60px",
          background:
            "linear-gradient(150deg, var(--forest) 0%, color-mix(in oklab, var(--forest) 78%, #04331A) 60%, #062a13 100%)"
        }}
      >
        <Rosette petals={ROSETTE_TOP} style={{ position: "absolute", right: "-12%", top: "8%", width: "78%", opacity: 0.16, filter: "blur(.3px)" }} />
        <Rosette petals={ROSETTE_BOTTOM} style={{ position: "absolute", left: "-18%", bottom: "-16%", width: "52%", opacity: 0.1 }} />

        <div style={{ position: "relative", display: "flex", alignItems: "center", gap: 14 }}>
          <Emblem size={56} />
          <div style={{ color: "#fff", fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 20, letterSpacing: "-.02em" }}>
            García&nbsp;Lorca
          </div>
        </div>

        <div style={{ position: "relative", color: "#fff", maxWidth: 460 }}>
          <div
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 8,
              padding: "7px 14px",
              borderRadius: 99,
              whiteSpace: "nowrap",
              background: "rgba(255,255,255,.14)",
              fontSize: 13.5,
              fontWeight: 600,
              marginBottom: 22
            }}
          >
            <span style={{ width: 8, height: 8, borderRadius: 99, background: "var(--lime)" }} />
            Ciclo escolar 2025–2026
          </div>
          <h1 style={{ fontSize: 46, lineHeight: 1.05, fontWeight: 800, color: "#fff" }}>
            Todo el colegio,
            <br />
            en un solo lugar.
          </h1>
          <p style={{ fontSize: 18, lineHeight: 1.5, color: "rgba(255,255,255,.82)", marginTop: 18, fontWeight: 400 }}>
            Kínder, Primaria, Secundaria y Preparatoria. Alumnos, finanzas, asistencia y calificaciones — siempre a un clic.
          </p>
        </div>

        <div style={{ position: "relative", display: "flex", gap: 10 }}>
          {["var(--lime)", "var(--forest)", "var(--yellow)", "var(--orange)", "var(--purple)"].map((c, i) => (
            <span key={i} style={{ width: 34, height: 6, borderRadius: 99, background: c, opacity: i === 0 ? 1 : 0.5 }} />
          ))}
        </div>
      </div>

      {/* form panel */}
      <div style={{ position: "relative", display: "grid", placeItems: "center", padding: "40px" }}>
        <div style={{ position: "absolute", top: 24, right: 28 }}>
          <ThemeToggle theme={theme} onToggle={onToggle} />
        </div>
        <div className="fade-up" style={{ width: "100%", maxWidth: 412 }}>
          <h2 style={{ fontSize: 30, fontWeight: 700, color: "var(--text)" }}>Bienvenido de nuevo 👋</h2>
          <p style={{ color: "var(--text-muted)", fontSize: 16, marginTop: 8 }}>Elige tu perfil para entrar.</p>

          {/* role segmented */}
          <div role="group" aria-label="Perfil" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10, marginTop: 26 }}>
            {ROLES.map((r) => {
              const on = role === r.id;
              return (
                <button
                  key={r.id}
                  type="button"
                  aria-pressed={on}
                  onClick={() => {
                    setRole(r.id);
                    setUser(PLACEHOLDER_USER[r.id]);
                  }}
                  style={{
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "center",
                    gap: 7,
                    padding: "15px 6px",
                    borderRadius: "var(--r-md)",
                    border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`,
                    background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)",
                    color: on ? "var(--forest)" : "var(--text-muted)",
                    transition: "all .18s var(--ease)",
                    fontFamily: "var(--font-display)",
                    fontWeight: 600,
                    fontSize: 13
                  }}
                >
                  <Icon name={r.icon} size={24} />
                  {r.label}
                </button>
              );
            })}
          </div>

          {/* fields */}
          <div style={{ marginTop: 22, display: "flex", flexDirection: "column", gap: 14 }}>
            <label style={{ display: "block" }}>
              <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 4 }}>Usuario</span>
              <div style={{ position: "relative", marginTop: 7 }}>
                <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}>
                  <Icon name="user" size={20} />
                </span>
                <input aria-label="Usuario" value={user} onChange={(e) => setUser(e.target.value)} style={inp} />
              </div>
            </label>
            <label style={{ display: "block" }}>
              <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 4 }}>Contraseña</span>
              <div style={{ position: "relative", marginTop: 7 }}>
                <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}>
                  <Icon name="shield" size={20} />
                </span>
                <input
                  aria-label="Contraseña"
                  type={show ? "text" : "password"}
                  value={pwd}
                  onChange={(e) => setPwd(e.target.value)}
                  style={{ ...inp, paddingRight: 48 }}
                />
                <button
                  type="button"
                  onClick={() => setShow((s) => !s)}
                  aria-label={show ? "Ocultar contraseña" : "Mostrar contraseña"}
                  aria-pressed={show}
                  style={{
                    position: "absolute",
                    right: 8,
                    top: "50%",
                    transform: "translateY(-50%)",
                    width: 36,
                    height: 36,
                    borderRadius: 99,
                    display: "grid",
                    placeItems: "center",
                    color: "var(--text-faint)"
                  }}
                >
                  <Icon name="eye" size={20} />
                </button>
              </div>
            </label>
          </div>

          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", margin: "16px 2px 22px" }}>
            <label style={{ display: "flex", alignItems: "center", gap: 9, fontSize: 14, color: "var(--text-muted)", cursor: "pointer" }}>
              <input type="checkbox" defaultChecked style={{ width: 18, height: 18, accentColor: "var(--lime)" }} />
              Recordarme
            </label>
            <a
              href="#"
              onClick={(e) => e.preventDefault()}
              style={{ fontSize: 14, fontWeight: 600, color: "var(--forest)", textDecoration: "none" }}
            >
              ¿Olvidaste tu contraseña?
            </a>
          </div>

          <Btn full icon="logout" onClick={() => onLogin(role)} style={{ height: 54, fontSize: 17 }}>
            Entrar al sistema
          </Btn>

          <p style={{ textAlign: "center", color: "var(--text-faint)", fontSize: 13, marginTop: 24 }}>
            ¿Problemas para entrar? Escribe a{" "}
            <span style={{ color: "var(--forest)", fontWeight: 600 }}>soporte@garcialorca.edu.mx</span>
          </p>
        </div>
      </div>
    </div>
  );
}

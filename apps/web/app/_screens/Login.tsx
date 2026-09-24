"use client";

import { useState, type CSSProperties, type ReactElement } from "react";
import { LogoBadge } from "../_design/Brand";
import { Btn, ThemeToggle } from "../_design/Controls";
import { Icon } from "../_design/Icon";
import { isComposing, lowercaseKeepingCaret } from "../_design/form/lowercaseKeepingCaret";
import { requestPasswordReset } from "../auth/api";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "";
import type { Theme } from "../theme/theme-script";

export interface LoginProps {
  theme: Theme;
  onToggle: () => void;
  onSubmit: (email: string, password: string) => Promise<void>;
  error?: string;
  pending?: boolean;
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

export function Login({ theme, onToggle, onSubmit, error, pending }: LoginProps): ReactElement {
  const [email, setEmail] = useState("");
  const [pwd, setPwd] = useState("");
  const [show, setShow] = useState(false);
  const [forgot, setForgot] = useState(false);
  const [forgotEmail, setForgotEmail] = useState("");
  const [sent, setSent] = useState(false);

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

        <div style={{ position: "relative", display: "flex", alignItems: "center" }}>
          <LogoBadge height={46} />
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
          <p style={{ color: "var(--text-muted)", fontSize: 16, marginTop: 8 }}>Ingresa con tu correo institucional.</p>

          {/* fields */}
          <div style={{ marginTop: 26, display: "flex", flexDirection: "column", gap: 14 }}>
            <label style={{ display: "block" }}>
              <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 4 }}>Correo</span>
              <div style={{ position: "relative", marginTop: 7 }}>
                <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}>
                  <Icon name="user" size={20} />
                </span>
                {/*
                  A minúsculas al teclear, como en el resto del sistema. Esta
                  pantalla no usa EmailField porque tiene su propio diseño con
                  ícono dentro del campo, pero la regla es la misma — y aquí
                  importa más que en ningún lado: el teclado que pone mayúscula
                  inicial deja a alguien fuera sin que entienda por qué.
                */}
                <input
                  aria-label="Correo"
                  type="text"
                  inputMode="email"
                  autoComplete="username"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  value={email}
                  onChange={(e) => setEmail(lowercaseKeepingCaret(e.target, isComposing(e)))}
                  onCompositionEnd={(e) => setEmail(lowercaseKeepingCaret(e.currentTarget, false))}
                  style={inp}
                />
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
                  autoComplete="current-password"
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
            {/* This was `href="#"` with preventDefault: an anchor that had
                never been connected to anything. A teacher who forgot their
                password had no way back in and the director could not help
                them either. */}
            <button
              type="button"
              onClick={() => { setForgot(true); setSent(false); }}
              style={{ fontSize: 14, fontWeight: 600, color: "var(--forest)", background: "transparent", border: "none", cursor: "pointer", padding: 0 }}
            >
              ¿Olvidaste tu contraseña?
            </button>
          </div>

          <Btn full icon="logout" onClick={() => void onSubmit(email.trim(), pwd)} disabled={pending} style={{ height: 54, fontSize: 17 }}>
            {pending ? "Entrando…" : "Entrar al sistema"}
          </Btn>
          {error ? (
            <p role="alert" style={{ color: "var(--orange)", fontSize: 14, marginTop: 12, textAlign: "center" }}>
              {error}
            </p>
          ) : null}

          {forgot ? (
            <ForgotPanel
              email={forgotEmail}
              sent={sent}
              onEmail={setForgotEmail}
              onSend={() => {
                // Fire and forget, and the screen says the same thing whatever
                // comes back. Reporting whether the address has an account
                // would turn this into a way to read the school's directory one
                // guess at a time.
                void requestPasswordReset(API_URL, forgotEmail.trim());
                setSent(true);
              }}
              onClose={() => setForgot(false)}
            />
          ) : null}

          <p style={{ textAlign: "center", color: "var(--text-faint)", fontSize: 13, marginTop: 24 }}>
            ¿Problemas para entrar? Escribe a{" "}
            <span style={{ color: "var(--forest)", fontWeight: 600 }}>soporte@garcialorca.edu.mx</span>
          </p>
        </div>
      </div>
    </div>
  );
}

/**
 * Asking for a reset link.
 *
 * The answer is ALWAYS the same sentence — "if that address has an account" —
 * and it is phrased that way on purpose. This is an unauthenticated form on a
 * system that holds minors' records; confirming that an address is registered
 * would make it a directory anyone could read, one guess at a time.
 *
 * It also tells people to check spam. The mail comes from a school's own
 * server, not a big sender, so some of it will land there and a person who is
 * not told to look will conclude nothing was sent — which is the failure this
 * whole change exists to end.
 */
function ForgotPanel({ email, sent, onEmail, onSend, onClose }: { email: string; sent: boolean; onEmail: (v: string) => void; onSend: () => void; onClose: () => void }): ReactElement {
  return (
    <div style={{ marginTop: 18, padding: "18px 18px 16px", borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface-3)" }}>
      {sent ? (
        <>
          <div style={{ fontWeight: 700, fontSize: 15, color: "var(--text)", marginBottom: 6 }}>Revisa tu correo</div>
          <p style={{ fontSize: 14, color: "var(--text-muted)", lineHeight: 1.5 }}>
            Si esa dirección tiene una cuenta, te llegará un correo en los próximos minutos. Revisa también tu carpeta de correo no deseado.
          </p>
          <div style={{ marginTop: 12 }}>
            <Btn kind="ghost" icon="back" onClick={onClose}>Volver</Btn>
          </div>
        </>
      ) : (
        <>
          <label htmlFor="forgot-email" style={{ display: "block", fontWeight: 700, fontSize: 15, color: "var(--text)", marginBottom: 6 }}>
            ¿Cuál es tu correo?
          </label>
          <p style={{ fontSize: 13.5, color: "var(--text-muted)", marginBottom: 10 }}>
            Te mandamos un enlace para que elijas una contraseña nueva.
          </p>
          <input
            id="forgot-email"
            type="text"
            inputMode="email"
            autoComplete="email"
            value={email}
            autoCapitalize="none"
            spellCheck={false}
            onChange={(e) => onEmail(lowercaseKeepingCaret(e.target, isComposing(e)))}
            onCompositionEnd={(e) => onEmail(lowercaseKeepingCaret(e.currentTarget, false))}
            style={{ width: "100%", height: 48, padding: "0 14px", borderRadius: "var(--r-md)", border: "1.5px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 15, outline: "none" }}
          />
          <div style={{ display: "flex", gap: 10, marginTop: 12 }}>
            <Btn icon="mail" onClick={onSend} disabled={email.trim() === ""}>Enviar enlace</Btn>
            <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          </div>
        </>
      )}
    </div>
  );
}

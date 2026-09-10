"use client";

import { type CSSProperties, type ReactElement, useState } from "react";

import { Icon } from "../_design/Icon";
import { Btn } from "../_design/Controls";

/**
 * Choosing your own password.
 *
 * This is the FIRST screen a new teacher ever sees of this system — before the
 * dashboard, before the login. It arrives from a mail sent by an address they
 * do not recognise, so its whole job in the first two seconds is to say "this
 * is your school", and only then ask for anything.
 *
 * Two design decisions worth their reasoning:
 *
 *   ONE password field, with a reveal toggle, not two with a confirmation.
 *   Somebody is typing on a phone keyboard, from a mail, standing up. Typing a
 *   password twice blind is the single biggest reason people abandon an
 *   activation screen, and the safety net that justified the confirmation — "if
 *   you typo it you are locked out forever" — is exactly what this change
 *   builds: they can ask for another link.
 *
 *   Requirements shown BEFORE typing, not after failing. A rule you learn by
 *   breaking it is a rule that cost somebody an attempt.
 */

const MIN_LEN = 8;

const field: CSSProperties = {
  width: "100%",
  height: 54,
  padding: "0 48px 0 16px",
  borderRadius: "var(--r-md)",
  border: "1.5px solid var(--border)",
  background: "var(--surface)",
  color: "var(--text)",
  fontSize: 16,
  outline: "none",
};

export interface SetPasswordProps {
  /** Shown so the person can confirm the link was meant for them. */
  email?: string;
  heading: string;
  intro: string;
  cta: string;
  pending: boolean;
  error?: string;
  onSubmit: (password: string) => void;
}

export function SetPassword({ email, heading, intro, cta, pending, error, onSubmit }: SetPasswordProps): ReactElement {
  const [password, setPassword] = useState("");
  const [show, setShow] = useState(false);
  const tooShort = password.length > 0 && password.length < MIN_LEN;
  const ready = password.length >= MIN_LEN && !pending;

  return (
    <div className="app-root" style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: "24px 20px" }}>
      <div style={{ width: "100%", maxWidth: 440 }}>
        {/* The school first. Somebody who has never heard of this system needs
            to know whose it is before they are asked to type anything. */}
        <div style={{ display: "flex", alignItems: "center", gap: 13, marginBottom: 26 }}>
          <div style={{ width: 52, height: 52, borderRadius: 16, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", flexShrink: 0 }}>
            <Icon name="shield" size={28} />
          </div>
          <div style={{ minWidth: 0 }}>
            <div style={{ fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 19, color: "var(--text)" }}>Colegio García Lorca</div>
            <div style={{ fontSize: 13.5, color: "var(--text-muted)" }}>Sistema de gestión escolar</div>
          </div>
        </div>

        <h1 style={{ fontFamily: "var(--font-display)", fontSize: 27, fontWeight: 800, color: "var(--text)", marginBottom: 8 }}>{heading}</h1>
        <p style={{ color: "var(--text-muted)", fontSize: 15, lineHeight: 1.5, marginBottom: email !== undefined ? 6 : 24 }}>{intro}</p>
        {email !== undefined ? (
          <p style={{ color: "var(--text-faint)", fontSize: 13.5, marginBottom: 24 }}>
            Para <span style={{ color: "var(--forest)", fontWeight: 600 }}>{email}</span>
          </p>
        ) : null}

        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (ready) onSubmit(password);
          }}
        >
          <label style={{ display: "block", fontSize: 14, fontWeight: 600, color: "var(--text-muted)", marginBottom: 7 }} htmlFor="pwd">
            Tu contraseña
          </label>
          {/* The rule, before it can be broken. */}
          <p id="pwd-hint" style={{ fontSize: 13, color: "var(--text-faint)", marginBottom: 8 }}>
            Al menos {MIN_LEN} caracteres.
          </p>
          <div style={{ position: "relative" }}>
            <input
              id="pwd"
              aria-describedby="pwd-hint"
              type={show ? "text" : "password"}
              autoComplete="new-password"
              autoFocus
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              style={field}
            />
            <button
              type="button"
              onClick={() => setShow((v) => !v)}
              aria-label={show ? "Ocultar contraseña" : "Mostrar contraseña"}
              aria-pressed={show}
              style={{ position: "absolute", right: 8, top: "50%", transform: "translateY(-50%)", width: 40, height: 40, borderRadius: 99, display: "grid", placeItems: "center", color: "var(--text-faint)", cursor: "pointer", background: "transparent", border: "none" }}
            >
              <Icon name="eye" size={20} />
            </button>
          </div>
          {tooShort ? (
            <p style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 8 }}>Te faltan {MIN_LEN - password.length} caracteres.</p>
          ) : null}

          <div style={{ marginTop: 22 }}>
            {/* type="submit", explicitly. Btn defaults to type="button", so
                without this the button sits inside the form and does nothing —
                the field fills, the click lands, and the page never moves. */}
            <Btn full type="submit" icon="check" disabled={!ready} style={{ height: 54, fontSize: 17 }}>
              {pending ? "Guardando…" : cta}
            </Btn>
          </div>
        </form>

        {error !== undefined ? (
          <p role="alert" tabIndex={-1} style={{ color: "var(--orange)", fontSize: 14.5, marginTop: 16, textAlign: "center" }}>
            {error}
          </p>
        ) : null}

        <p style={{ textAlign: "center", color: "var(--text-faint)", fontSize: 13, marginTop: 26 }}>
          ¿Problemas para entrar? Escribe a{" "}
          <span style={{ color: "var(--forest)", fontWeight: 600 }}>soporte@garcialorca.edu.mx</span>
        </p>
      </div>
    </div>
  );
}
